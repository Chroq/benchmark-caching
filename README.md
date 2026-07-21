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

Le projet implémente et compare 4 configurations de moteurs de cache :

### A. In-Memory (Otter Cache)

- **Description :** Stockage en mémoire vive Go via le moteur d'in-memory caching `Otter` (supportant la gestion dynamique du TTL).
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

### D. Optimized PostgreSQL

- **Description :** Configuration tirant parti des fonctionnalités avancées de PostgreSQL pour simuler un comportement de cache transitoire ultra-performant.
- **Optimisations implémentées :**
  1.  **Table `UNLOGGED` :** Désactivation du journal de transactions (WAL - Write-Ahead Logging). Élimine les goulots d'étranglement d'I/O disque liés aux écritures WAL (vitesse proche du in-memory).
  2.  **Purge Asynchrone Non-Bloquante par Lots :** Invalidation via une fonction PL/pgSQL (`purge_expired_cache_keys`) exécutée régulièrement par `pg_cron` qui supprime les clés expirées par lots (`DELETE` par tranches de 10 000 avec `FOR UPDATE SKIP LOCKED`), préservant la fluidité des lectures (`GET`).
  3.  **Facteur de Remplissage (`fillfactor = 70`) :** La table réserve 30% d'espace libre par page physique pour favoriser les mises à jour en place (mécanisme _HOT_ - Heap-Only Tuple) sans réécrire les index.
  4.  **Clés Binaires UUID/ULID :** Clé primaire `key UUID` indexée directement sur 16 octets réels pour des recherches B-Tree instantanées (< 1ms).
  5.  **Sérialisation Protobuf Sans Réflexion :** Les objets Go sont sérialisés au format Protobuf binaire avec zéro allocation mémoire.

---

## 3. Prérequis & Préparation du Système

### Prérequis Logiciels

- **Go** (version 1.26+)
- **Valkey** (version 9.1.0+) ou Redis
- **PostgreSQL** (version 18.4+)
- **Vegeta** (outil d'invalidation/charge HTTP, installable via `go install github.com/tsenart/vegeta/v12@latest`)

### Isolation des Ressources (Contention CPU/RAM)

Pour garantir une comparaison équitable, le serveur Go, PostgreSQL et Valkey doivent être limités en ressources système sous Linux :

```bash
# Limiter PostgreSQL à 2 cœurs CPU et 4 Go de RAM
sudo systemctl set-property postgresql CPUQuota=200%
sudo systemctl set-property postgresql MemoryMax=4G

# Limiter Valkey à 1 cœur CPU et 4 Go de RAM
sudo systemctl set-property valkey-server CPUQuota=100%
sudo systemctl set-property valkey-server MemoryMax=4G
```

### Optimisation Réseau (TIME_WAIT Exhaustion)

Lors de tests de charge massifs, l'outil de benchmark (`vegeta`) ouvre et ferme des millions de sockets TCP.

Pour recycler instantanément ces sockets, configurez les paramètres réseau de votre noyau Linux :

```bash
make tune-os
```

---

## 4. Initialisation de la Base de Données & Génération des Cibles

Avant de lancer le serveur, créez le schéma de base de données et préparez les jeux de données :

```bash
# Pour le mode Standard PostgreSQL
psql -U postgres -d postgres -f internal/infrastructure/postgresql/standard/schema.sql

# Pour le mode Optimized PostgreSQL
psql -U postgres -d postgres -f internal/infrastructure/postgresql/optimized/schema.sql
```

### Génération du Fichier de Clés & Hydratation de Vegeta

Un outil dédié (`cmd/gentargets`) génère un fichier `keys.txt` rempli de 100 000 clés ULID/UUID ainsi que les fichiers de cibles Vegeta (`targets_*_get.txt` et `targets_*_set.txt`) :

```bash
make gen-targets
```

---

## 5. Exécution des Benchmarks

### Utilisation Rapide via le Makefile

Le Makefile gère la compilation, la génération des cibles, l'arrêt/démarrage des services, le préchauffage, et le benchmark HTTP via **Vegeta** :

```bash
# Compiler le serveur Go
make build

# Exécuter le benchmark rapide de validation (5 secondes par test)
make quick

# Exécuter le benchmark intermédiaire (60 secondes par test)
make medium

# Exécuter le benchmark officiel d'endurance (10 minutes par test)
make long
```

### Lancement Manuel du Serveur & de Vegeta

```bash
# Lancer le serveur
./bin/server -engine optimized-postgresql

# Dans un autre terminal, exécuter l'attaque Vegeta avec le fichier de cibles
vegeta attack -rate=0 -workers=250 -duration=60s -targets=targets_postgres_get.txt | vegeta report
```

_Endpoints HTTP exposés sur `:8080` :_

- GET : `/memory/get`, `/valkey/get`, `/postgres/get`
- POST : `/memory/set`, `/valkey/set`, `/postgres/set`
