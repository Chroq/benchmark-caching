# Benchmark Caching

Ce projet a pour but de comparer de manière objective les performances de **PostgreSQL** (sous deux configurations : Naïve et Optimisée) en tant que cache Key-Value haute performance face à **Valkey** et une implémentation **In-Memory** native.

---

## Architecture & Optimisations

1. **In-Memory** : Utilisation d'une structure `sync.Map` globale stockant directement les pointeurs des objets sans surcoût de sérialisation.
2. **Valkey** : Connexion via un pool de 500 connexions actives. Sérialisation optimisée au format standard Protobuf Wire (écrit manuellement à la main, sans réflexion) et stockage via des clés ULID en chaînes de caractères Crockford Base32.
3. **Naive PostgreSQL** : 
   - Table classique SQL (`cache_naive`).
   - Clé primaire indexée en `VARCHAR(255)`.
   - Sérialisation au format standard **JSON** via le package `encoding/json` standard de Go.
4. **Optimized PostgreSQL** :
   - Table `UNLOGGED` (sans écriture de WAL) partitionnée par tranches de 1 heure (`cache_optimized_partitioned`).
   - `fillfactor = 70` sur les tables filles pour limiter le coût des HOT (Heap-Only Tuple) updates.
   - Clé de type `UUID` (16 octets bruts pour stocker l'ULID).
   - Sérialisation manuelle hautement performante au format **Protobuf** (zéro allocation, zéro réflexion).
   - Utilisation de **requêtes préparées globales** stockées au niveau de la connexion du pool (`pgxpool.Config.AfterConnect`) pour éliminer tout surcoût de parsing SQL par PostgreSQL lors du bench.

---

## Prérequis

- **Go 1.26+**
- **Valkey 9.1.0+**
- **PostgreSQL 18.4+**

---

## Configuration & Préparation

### Limitation des ressources (Contention CPU/RAM)
Pour garantir l'équité, limitez PostgreSQL et Valkey sur votre machine :

#### PostgreSQL
```bash
sudo systemctl set-property postgresql CPUQuota=200%
sudo systemctl set-property postgresql MemoryMax=2G
```

#### Valkey
```bash
sudo systemctl set-property valkey-server CPUQuota=200%
sudo systemctl set-property valkey-server MemoryMax=2G
```

### La saturation des ports TCP de Linux (TIME_WAIT)

C'est un goulot d'étranglement purement lié à l'OS (Linux) lorsque Bombardier bombarde pendant une longue durée.
À 60 000 requêtes/seconde pendant 600 secondes, Bombardier va ouvrir et fermer des millions de connexions TCP secondaires. Même avec le protocole Keep-Alive, Linux garde les sockets fermés dans un état appelé TIME_WAIT pendant 60 secondes par sécurité avant de libérer le port local. Vous risquez d'atteindre la limite maximale de ports de votre OS au bout de 3 ou 4 minutes, ce qui provoquera des erreurs de connexion.

Pour éviter ce bruit réseau sur un test long, exécutez ces deux commandes dans votre terminal Linux avant de lancer le benchmark pour forcer l'OS à recycler immédiatement les sockets :

```bash
sudo sysctl -w net.ipv4.tcp_tw_reuse=1
sudo sysctl -w net.ipv4.ip_local_port_range="1024 65535"
```

---

## Initialisation de la Base de Données

Avant de lancer les benchmarks PostgreSQL, initialisez le schéma et les procédures de maintenance automatique :

```bash
psql -U postgres -d postgres -f schema.sql
```

---

## Lancement du Serveur de Benchmark

Le serveur HTTP s'exécute sur le port `:8080` et charge au démarrage **10 000 entrées fictives** dans le moteur ciblé à l'aide d'un flag d'exécution `-engine`.

```bash
# Pour tester Valkey (pré-remplissage via Pipeline)
go run cmd/main.go -engine valkey

# Pour tester PostgreSQL Optimisé (pré-remplissage via Transaction)
go run cmd/main.go -engine optimized-postgresql

# Pour tester PostgreSQL Naïf
go run cmd/main.go -engine naive-postgresql

# Pour tester la sync.Map In-Memory
go run cmd/main.go -engine memory
```

### Variables d'environnement configurables :
- `PORT` : Port d'écoute du serveur HTTP (défaut : `8080`).
- `DATABASE_URL` : URL de connexion PostgreSQL (défaut : `postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable`).
- `VALKEY_URL` : Adresse de connexion Valkey/Redis (défaut : `localhost:6379`).

---

## Exécution des Benchmarks (Exemple avec bombardier)

Les handlers ne font aucun log console pour éviter de saturer les entrées/sorties (I/O) de l'hôte et brider le serveur. Chaque requête HTTP pioche de manière non-bloquante (grâce à un pool `sync.Pool` de générateurs `math/rand`) un ID aléatoire parmi les 10 000 générés au démarrage.

### 1. In-Memory
```bash
# GET requests
bombardier -c 500 -d 30s http://localhost:8080/memory/get

# SET requests (POST)
bombardier -m POST -f payload.json -c 500 -d 30s http://localhost:8080/memory/set
```

### 2. Valkey
```bash
# GET requests
bombardier -c 500 -d 30s http://localhost:8080/valkey/get

# SET requests (POST)
bombardier -m POST -f payload.json -c 500 -d 30s http://localhost:8080/valkey/set
```

### 3. PostgreSQL Naïf
Avant de lancer le test, démarrez le serveur avec : `go run cmd/main.go -engine naive-postgresql`
```bash
# GET requests
bombardier -c 500 -d 30s http://localhost:8080/postgres/get

# SET requests (POST)
bombardier -m POST -f payload.json -c 500 -d 30s http://localhost:8080/postgres/set
```

### 4. PostgreSQL Optimisé
Avant de lancer le test, démarrez le serveur avec : `go run cmd/main.go -engine optimized-postgresql`
```bash
# GET requests
bombardier -c 500 -d 30s http://localhost:8080/postgres/get

# SET requests (POST)
bombardier -m POST -f payload.json -c 500 -d 30s http://localhost:8080/postgres/set
```

---

## 5. Benchmark de Référence Direct (Pilote PostgreSQL de Go)

Si vous souhaitez éliminer toute trace du réseau et du serveur HTTP pour évaluer uniquement la vitesse pure des pilotes et de PostgreSQL (Naïf vs Optimisé), vous pouvez exécuter le benchmark de pilote Go en parallèle :

```bash
make bench-postgres-direct
```

Ce benchmark génère en mémoire 1 000 clés et exécute en parallèle les opérations de lecture/écriture directement dans les tables PostgreSQL.

