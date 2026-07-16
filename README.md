# Benchmark Caching: PostgreSQL vs Valkey vs In-Memory

Ce projet propose une étude comparative la plus rigoureuse possible des performances de **PostgreSQL** utilisé comme cache Key-Value, face à **Valkey** (le fork open-source de Redis) et à une implémentation **In-Memory** native en Go.

---

## 1. Problématique & Objectif

Dans les architectures modernes, l'ajout d'une base Key-Value comme Redis/Valkey est le réflexe standard pour le caching. Cependant, on constate une complexité opérationnelle : nouveaux serveurs à monitorer, synchronisation réseau, coût d'infrastructure et gestion de la cohérence des données.

**L'objectif de ce projet est de répondre à la question suivante :**

> _Est ce que PostgreSQL peut être utilisé comme moteur de cache haute performance ?_

Pour ce faire, nous mesurons le débit (MB/s), le débit transactionnel (requêtes/seconde) et la latence sous forte contention lors d'un test d'endurance de 10 minutes.

---

## 2. Architecture & Stratégies d'Optimisation

Le projet implémente et compare 5 configurations de moteurs de cache :

### A. In-Memory (Go `sync.Map`)

- **Description :** Stockage en mémoire vive Go via la structure standard `sync.Map`.
- **Caractéristique :** Zéro réseau, zéro sérialisation (stockage direct de pointeurs d'objets). Représente un indicateur de performance de référence.

### B. Valkey

- **Description :** Cache Key-Value autonome connecté via un pool de 500 connexions actives.
- **Optimisations :**
  - Sérialisation manuelle optimisée au format **Protobuf Wire** standard (écriture et lecture directes sur octets, sans utilisation du package de réflexion Go).
  - Encodage des clés au format ULID chaîné en base32 (Crockford).

### C. Standard PostgreSQL (Flat)

- **Description :** Modèle relationnel classique où chaque champ de l'objet est stocké dans une colonne dédiée (`users_standard`), plutôt que dans un blob JSON/Protobuf.
- **Optimisations :**
  - Clé de type binaire native `UUID` (16 octets).
  - Requêtes préparées au niveau de la connexion pour éviter le parsing SQL à chaque appel.
  - Depuis la version 18 de PostgreSQL, l'usage du cache a été très largement étendu.

### D. Optimized PostgreSQL

- **Description :** Configuration tirant parti des fonctionnalités avancées de PostgreSQL 18+ pour simuler un comportement de cache transient.
- **Optimisations implémentées :**
  1.  **Tables `UNLOGGED` :** Désactivation du journal de transactions (WAL - Write-Ahead Logging). En cas de crash, la table est vidée, ce qui est le comportement attendu d'un cache, éliminant ainsi les goulots d'étranglement d'I/O disque liés aux écritures WAL.
  2.  **Partitionnement Temporel Automatique :** La table parent `cache_optimized_partitioned` est découpée en partitions physiques de 1 heure. L'invalidation ou l'expiration des clés se fait en supprimant directement une partition entière via une commande `DROP TABLE` (gérée automatiquement), éliminant les opérations `DELETE` massives et le besoin d'exécuter des `VACUUM` coûteux.
  3.  **Facteur de Remplissage (`fillfactor = 70`) :** Les tables filles réservent 30% d'espace libre par page de base de données. Cela permet aux opérations de mise à jour (`UPDATE`) de s'insérer dans la même page physique (mécanisme _HOT - Heap-Only Tuple_), évitant la réécriture d'index.
  4.  **Clés Binaires UUID :** Clé primaire indexée sur 16 octets réels (`UUID`), réduisant drastiquement la taille des index comparé au stockage `VARCHAR`.
  5.  **Sérialisation Protobuf Sans Réflexion :** Les objets Go sont sérialisés manuellement au format Protobuf binaire avec zéro allocation mémoire.
  6.  **Requêtes Préparées Globales :** Enregistrement des requêtes SQL lors de la connexion initiale (`pgxpool.Config.AfterConnect`) pour contourner la phase de planification de requêtes de Postgres.

---

## 3. Prérequis & Préparation du Système

### Prérequis Logiciels

- **Go** (version 1.26+)
- **Valkey** (version 9.1.0+) ou Redis
- **PostgreSQL** (version 18.4+)

### Isolation des Ressources (Contention CPU/RAM)

Pour garantir une comparaison équitable, le serveur Go, PostgreSQL et Valkey doivent être limités en ressources système. Sous Linux, exécutez ces commandes pour configurer des quotas système :

```bash
# Limiter PostgreSQL à 2 cœurs CPU et 2 Go de RAM
sudo systemctl set-property postgresql CPUQuota=200%
sudo systemctl set-property postgresql MemoryMax=2G

# Limiter Valkey à 1 cœurs CPU (Valkey est monothreadé) et 2 Go de RAM
sudo systemctl set-property valkey-server CPUQuota=100%
sudo systemctl set-property valkey-server MemoryMax=2G
```

### Optimisation Réseau (TIME_WAIT Exhaustion)

Lors de tests de charge massifs à haute fréquence (60 000+ reqs/sec), l'outil de benchmark (`bombardier`) ouvre et ferme des millions de sockets TCP. Linux garde ces sockets fermés dans un état de sécurité `TIME_WAIT` pendant 60 secondes, saturant rapidement les ports éphémères de l'OS.

Pour recycler instantanément ces sockets, configurez les paramètres réseau de votre noyau Linux :

```bash
make tune-os
# Ou manuellement :
# sudo sysctl -w net.ipv4.tcp_tw_reuse=1
# sudo sysctl -w net.ipv4.ip_local_port_range="1024 65535"
```

---

## 4. Initialisation de la Base de Données

Avant de lancer le serveur, vous devez créer le schéma de base de données et charger la fonction de gestion des partitions automatiques dans PostgreSQL.

```bash
psql -U postgres -d postgres -f schema.sql
```

> [!NOTE]
> Le fichier `schema.sql` crée automatiquement les partitions pour les 8 prochaines heures et configure une tâche récurrente (si l'extension `pg_cron` est présente) pour créer les futures partitions et supprimer proprement les anciennes.

---

## 5. Exécution des Benchmarks

Le serveur HTTP est écrit avec le framework ultra-rapide `fasthttp`. Au démarrage, il pré-charge **10 000 000 de clés fictives** dans le moteur spécifié afin d'avoir un jeu de données représentatif de 10 millions d'entrées.

### Utilisation Rapide via le Makefile

Le projet intègre un Makefile qui gère le cycle de vie complet des tests (compilation, arrêt/démarrage des services pour isolation, préchauffage du cache, exécution des requêtes GET puis SET, et nettoyage).

```bash
# Compiler le serveur Go optimisé pour la production
make build

# Exécuter le benchmark rapide de validation (5 secondes par test)
make quick

# Exécuter le benchmark intermédiaire (60 secondes par test)
make medium

# Exécuter le benchmark officiel d'endurance (10 minutes par test)
make long
```

### Lancement Manuel du Serveur

Vous pouvez aussi démarrer le serveur individuellement en fonction du moteur choisi :

```bash
# Tester Valkey
go run cmd/main.go -engine valkey

# Tester PostgreSQL Optimisé
go run cmd/main.go -engine optimized-postgresql

# Tester PostgreSQL Standard
go run cmd/main.go -engine standard-postgresql

# Tester la mémoire vive native Go
go run cmd/main.go -engine memory
```

_Endpoints HTTP exposés sur `:8080` :_

- GET : `/memory/get`, `/valkey/get`, `/postgres/get`
- POST : `/memory/set`, `/valkey/set`, `/postgres/set`

## 6. Résultats Détaillés (Test d'Endurance de 10 Minutes)

Voici les métriques de performance extraites des logs réels d'endurance de **10 minutes** sous une charge constante de **250 connexions simultanées** (WARMUP de 30 secondes préalable pour stabiliser les caches).

### Tableau de Synthèse des Performances

| Configuration              | Opération | Reqs/Sec (Moyen) | Latence (Moyenne) | Latence p50 | Latence p99 | Débit (MB/s) | Erreurs (5xx / Timeouts) |
| :------------------------- | :-------: | :--------------: | :---------------: | :---------: | :---------: | :----------: | :----------------------: |
| **In-Memory (`sync.Map`)** |  **GET**  |  **174 386,79**  |      1,43 ms      |   1,17 ms   |   5,42 ms   |  27,44 MB/s  |          0 / 0           |
|                            |  **SET**  |  **113 974,34**  |      2,19 ms      |   1,81 ms   |   7,88 ms   |  24,78 MB/s  |          0 / 0           |
| **Valkey**                 |  **GET**  |  **77 915,04**   |      3,21 ms      |   3,04 ms   |   7,21 ms   |  12,26 MB/s  |          0 / 0           |
|                            |  **SET**  |  **70 096,91**   |      3,56 ms      |   3,45 ms   |   7,26 ms   |  15,24 MB/s  |          0 / 0           |
| **PostgreSQL Standard**    |  **GET**  |  **39 156,35**   |      6,39 ms      |   6,20 ms   |  12,34 ms   |  6,23 MB/s   |          0 / 0           |
|                            |  **SET**  |  **11 834,16**   |     21,13 ms      |  13,25 ms   |  128,34 ms  |  2,60 MB/s   |          0 / 0           |
| **PostgreSQL Optimisé**    |  **GET**  |  **40 115,05**   |      6,23 ms      |   6,03 ms   |  12,51 ms   |  6,38 MB/s   |          0 / 0           |
|                            |  **SET**  |  **37 764,33**   |      6,62 ms      |   6,49 ms   |  11,74 ms   |  8,28 MB/s   |          0 / 0           |

---

## 7. Analyse Technique & Enseignements

### 1. La métamorphose de PostgreSQL (Standard vs Optimisé)

- **Lectures (GET) :** Les optimisations maintiennent PostgreSQL à un niveau extrêmement performant et stable de **40 115 reqs/s** avec une latence moyenne de **6,23 ms** (contre **39 156 reqs/s** et **6,39 ms** pour le mode standard).
- **Écritures (SET) :** C'est le gain le plus spectaculaire. PostgreSQL Optimisé atteint **37 764 reqs/s**, soit **3,2x plus rapide** que le mode Standard (**11 834 reqs/s**), grâce aux tables `UNLOGGED` et au partitionnement temporel qui éliminent l'amplification d'écriture.

### 3. Les clés du succès de la configuration Optimisée

- **L'absence de WAL (`UNLOGGED`) :** En se débarrassant des écritures disques synchrones du WAL, PostgreSQL se comporte comme un vrai moteur In-Memory persistant uniquement sur pages mémoires partagées (_shared buffers_).
- **Le partitionnement par heure :** Au lieu d'avoir un index gigantesque sur 10 millions de clés qui ralentit au fil des écritures, chaque heure cible une table physique isolée et petite. La purge automatique d'une partition obsolète est instantanée (`DROP TABLE`), évitant tout besoin de nettoyage en arrière-plan.
- **Protobuf sans allocation :** L'encodage binaire strict réduit la taille du payload stocké dans la colonne `BYTEA` et décharge complètement le processeur Go de la réflexion.

### 4. PostgreSQL face à Valkey (Redis)

- En lecture (GET), PostgreSQL Optimisé (40.1k reqs/s) offre **51% des performances de Valkey** (77.9k reqs/s).
- En écriture (SET), PostgreSQL Optimisé (37.7k reqs/s) offre **53% des performances de Valkey** (70.0k reqs/s).

**Conclusion architecturale :**
Si vos besoins de cache se situent en dessous de 40 000 requêtes/seconde par nœud d'application, et que vous possédez déjà une base PostgreSQL dans votre infrastructure, il est **techniquement viable** d'utiliser PostgreSQL avec cette configuration optimisée (table `UNLOGGED` + partitionnement). Vous éliminez ainsi le coût opérationnel lié au déploiement et à la maintenance d'un cluster Redis/Valkey dédié.
