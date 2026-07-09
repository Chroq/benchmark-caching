package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	httpAdapter "github.com/Chroq/benchmark-caching/adapters/http"
	dbAdapter "github.com/Chroq/benchmark-caching/adapters/postgres"
	valkeyAdapter "github.com/Chroq/benchmark-caching/adapters/valkey"
	"github.com/Chroq/benchmark-caching/domain"
	"github.com/Chroq/benchmark-caching/ports"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/valyala/fasthttp"
)

const nbKey = 10_000_000

func main() {
	domain.BootstrapTime = time.Now().UTC()

	// 1. Define command line flags
	engine := flag.String("engine", "optimized-postgresql", "Storage engine to benchmark (memory, valkey, naive-postgresql, optimized-postgresql, standard-postgresql)")
	logLevel := flag.String("log-level", "info", "Log level (debug, info, warn, error, production)")
	flag.Parse()

	// 2. Configure structured logging
	var level slog.Level
	switch *logLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	case "production":
		level = slog.LevelWarn // Suppress Info/Debug logs in production to optimize throughput
	default:
		level = slog.LevelInfo
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	}))
	slog.SetDefault(logger)

	slog.Info("Benchmarking engine selected", "engine", *engine, "logLevel", *logLevel)

	// 3. Load configurations
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"
	}

	valkeyURL := os.Getenv("VALKEY_URL")
	if valkeyURL == "" {
		valkeyURL = "localhost:6379"
	}

	// 4. Generate the global slice of distinct ULIDs and dummy users
	slog.Info("Generating dummy ULID-based cache entries...", "count", nbKey)
	globalKeys := make([][16]byte, nbKey)
	dummyUsers := make([]domain.UserData, nbKey)
	now := time.Now().Unix()

	for i := range nbKey {
		ulid := generateDeterministicULID(i)
		globalKeys[i] = ulid

		dummyUsers[i] = domain.UserData{
			ID:        ulid,
			FirstName: "Jean-Sébastien",
			LastName:  "Bach",
			BirthDate: -6468729600, // 1685-03-31
			Active:    true,
			CreatedAt: now,
			UpdatedAt: now,
			DeletedAt: 0,
		}
	}

	var naiveRepo ports.NaiveUserRepository
	var optRepo ports.OptimizedUserRepository
	var stdRepo ports.StandardUserRepository
	var valkeyRepo *valkeyAdapter.Repository
	var memoryStore *sync.Map

	// 5. Initialize storage engines & perform Seeding based on active selection
	ctx := context.Background()

	// Clean GC to get a known starting point
	runtime.GC()

	switch *engine {
	case "memory":
		slog.Info("Initializing In-Memory sync.Map...")
		memoryStore = &sync.Map{}
		for i := 0; i < nbKey; i++ {
			memoryStore.Store(globalKeys[i], &dummyUsers[i])
		}

	case "valkey":
		slog.Info("Initializing Valkey Client Pool (PoolSize=500)...")
		rdb := redis.NewClient(&redis.Options{
			Addr:         valkeyURL,
			PoolSize:     500,
			ReadTimeout:  5 * time.Minute,
			WriteTimeout: 5 * time.Minute,
		})

		var pingErr error
		for i := 0; i < 5; i++ {
			pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			pingErr = rdb.Ping(pingCtx).Err()
			cancel()
			if pingErr == nil {
				break
			}
			slog.Info("Valkey connection failed, retrying in 1s...", "attempt", i+1, "error", pingErr)
			time.Sleep(1 * time.Second)
		}
		if pingErr != nil {
			slog.Error("Valkey connection failed after retries", "error", pingErr)
			os.Exit(1)
		}

		valkeyRepo = valkeyAdapter.NewRepository(rdb)

		// Check if the database is fully populated and not expired
		dbSize, errSize := rdb.DBSize(ctx).Result()
		firstKeyStr := domain.EncodeULID(globalKeys[0])
		ttl, errTTL := rdb.TTL(ctx, firstKeyStr).Result()

		if errSize == nil && dbSize == int64(nbKey) && errTTL == nil && ttl > 0 {
			slog.Info("Valkey DB already fully populated and valid. Skipping seeding.", "ttl_seconds", ttl.Seconds())
		} else {
			slog.Info("Valkey DB is empty, expired, or mismatch. Flushing and seeding...", "dbSize", dbSize, "ttl", ttl)
			_ = rdb.FlushDB(ctx).Err()
			slog.Info("Seeding records into Valkey via Pipeline...")
			seedCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
			if err := valkeyRepo.Seed(seedCtx, dummyUsers, 100*time.Hour); err != nil {
				cancel()
				slog.Error("Valkey seeding failed", "error", err)
				os.Exit(1)
			}
			cancel()
		}

	case "naive-postgresql", "optimized-postgresql", "standard-postgresql":
		slog.Info("Connecting to PostgreSQL to run migrations...")
		var migConn *pgx.Conn
		var err error
		for i := 0; i < 5; i++ {
			connectCtx, connectCancel := context.WithTimeout(ctx, 5*time.Second)
			migConn, err = pgx.Connect(connectCtx, dbURL)
			connectCancel()
			if err == nil {
				break
			}
			slog.Info("PostgreSQL connection failed, retrying in 1s...", "attempt", i+1, "error", err)
			time.Sleep(1 * time.Second)
		}
		if err != nil {
			slog.Error("PostgreSQL migration connection failed after retries", "error", err)
			os.Exit(1)
		}

		slog.Info("Executing database migrations from schema.sql...")
		schemaBytes, err := os.ReadFile("schema.sql")
		if err != nil {
			schemaBytes, err = os.ReadFile("../schema.sql")
			if err != nil {
				migConn.Close(context.Background())
				slog.Error("Failed to read schema.sql", "error", err)
				os.Exit(1)
			}
		}

		execCtx, execCancel := context.WithTimeout(ctx, 60*time.Second)
		_, err = migConn.Exec(execCtx, string(schemaBytes))
		execCancel()
		if err != nil {
			migConn.Close(context.Background())
			slog.Error("Failed to execute database migrations", "error", err)
			os.Exit(1)
		}
		migConn.Close(context.Background())
		slog.Info("Database migrations executed successfully.")

		slog.Info("Initializing PostgreSQL Pool (MaxConns=25, MinConns=25)...")
		config, err := pgxpool.ParseConfig(dbURL)
		if err != nil {
			slog.Error("Failed to parse DATABASE_URL", "error", err)
			os.Exit(1)
		}

		config.MaxConns = 25
		config.MinConns = 25
		config.MaxConnIdleTime = 30 * time.Minute
		config.MaxConnLifetime = 5 * time.Minute

		config.AfterConnect = func(connectCtx context.Context, conn *pgx.Conn) error {
			// Prepare Naive Statements
			_, errNaiveGet := conn.Prepare(connectCtx, "get_user_naive",
				"SELECT value FROM cache_naive WHERE key = $1 AND expires_at > $2")
			_, errNaiveSet := conn.Prepare(connectCtx, "set_user_naive",
				"INSERT INTO cache_naive (key, value, expires_at) VALUES ($1, $2, $3) ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, expires_at = EXCLUDED.expires_at")

			// Prepare Standard Statements
			_, errStdGet := conn.Prepare(connectCtx, "get_user_standard",
				"SELECT id, first_name, last_name, birth_date, active, created_at, updated_at, deleted_at FROM users_standard WHERE id = $1 AND expires_at > $2")
			_, errStdSet := conn.Prepare(connectCtx, "set_user_standard",
				"INSERT INTO users_standard (id, first_name, last_name, birth_date, active, created_at, updated_at, deleted_at, expires_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) ON CONFLICT (id) DO UPDATE SET first_name = EXCLUDED.first_name, last_name = EXCLUDED.last_name, birth_date = EXCLUDED.birth_date, active = EXCLUDED.active, updated_at = EXCLUDED.updated_at, deleted_at = EXCLUDED.deleted_at, expires_at = EXCLUDED.expires_at")

			if errNaiveGet != nil {
				return errNaiveGet
			}
			if errNaiveSet != nil {
				return errNaiveSet
			}
			if errStdGet != nil {
				return errStdGet
			}
			if errStdSet != nil {
				return errStdSet
			}
			return nil
		}

		pool, err := pgxpool.NewWithConfig(ctx, config)
		if err != nil {
			slog.Error("PostgreSQL pool setup failed", "error", err)
			os.Exit(1)
		}
		defer pool.Close()

		connCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		if err := pool.Ping(connCtx); err != nil {
			cancel()
			slog.Error("PostgreSQL connection failed", "error", err)
			os.Exit(1)
		}
		cancel()

		switch *engine {
		case "naive-postgresql":
			naiveRepo = dbAdapter.NewNaiveRepository(pool)
			var exists bool
			err := pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM cache_naive)").Scan(&exists)
			if err == nil && exists {
				// Check if there are any expired keys
				var hasExpired bool
				errExists := pool.QueryRow(ctx, 
					"SELECT EXISTS(SELECT 1 FROM cache_naive WHERE expires_at <= $1)",
					time.Now().UTC(),
				).Scan(&hasExpired)

				if errExists == nil && !hasExpired {
					slog.Info("PostgreSQL cache_naive table already populated and valid. Skipping seeding.")
					slog.Info("Analyzing cache_naive table...")
					_, _ = pool.Exec(ctx, "ANALYZE cache_naive")
				} else {
					slog.Info("PostgreSQL cache_naive table has expired keys. Extending TTL of expired keys...")
					_, errExt := pool.Exec(ctx, 
						"UPDATE cache_naive SET expires_at = $1 WHERE expires_at <= $2",
						time.Now().UTC().Add(100*time.Hour),
						time.Now().UTC(),
					)
					if errExt != nil {
						slog.Error("Failed to extend naive TTL", "error", errExt)
					}
					slog.Info("Analyzing cache_naive table...")
					_, _ = pool.Exec(ctx, "ANALYZE cache_naive")
				}
			} else {
				slog.Info("PostgreSQL cache_naive count mismatch or missing. Truncating table...", "expected", nbKey)
				_, _ = pool.Exec(ctx, "TRUNCATE TABLE cache_naive")
				slog.Info("Seeding records into cache_naive...")
				seedCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
				if err := naiveRepo.Seed(seedCtx, dummyUsers, 100*time.Hour); err != nil {
					cancel()
					slog.Error("PostgreSQL naive seeding failed", "error", err)
					os.Exit(1)
				}
				cancel()
				slog.Info("Analyzing cache_naive table...")
				_, _ = pool.Exec(ctx, "ANALYZE cache_naive")
			}
		case "standard-postgresql":
			stdRepo = dbAdapter.NewStandardRepository(pool)
			var exists bool
			err := pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM users_standard)").Scan(&exists)
			if err == nil && exists {
				// Check if there are any expired keys
				var hasExpired bool
				errExists := pool.QueryRow(ctx, 
					"SELECT EXISTS(SELECT 1 FROM users_standard WHERE expires_at <= $1)",
					time.Now().UTC(),
				).Scan(&hasExpired)

				if errExists == nil && !hasExpired {
					slog.Info("PostgreSQL users_standard table already populated and valid. Skipping seeding.")
					slog.Info("Analyzing users_standard table...")
					_, _ = pool.Exec(ctx, "ANALYZE users_standard")
				} else {
					slog.Info("PostgreSQL users_standard table has expired keys. Extending TTL of expired keys...")
					_, errExt := pool.Exec(ctx, 
						"UPDATE users_standard SET expires_at = $1 WHERE expires_at <= $2",
						time.Now().UTC().Add(100*time.Hour),
						time.Now().UTC(),
					)
					if errExt != nil {
						slog.Error("Failed to extend standard TTL", "error", errExt)
					}
					slog.Info("Analyzing users_standard table...")
					_, _ = pool.Exec(ctx, "ANALYZE users_standard")
				}
			} else {
				slog.Info("PostgreSQL users_standard count mismatch or missing. Truncating table...", "expected", nbKey)
				_, _ = pool.Exec(ctx, "TRUNCATE TABLE users_standard")
				slog.Info("Seeding records into users_standard...")
				seedCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
				if err := stdRepo.Seed(seedCtx, dummyUsers, 100*time.Hour); err != nil {
					cancel()
					slog.Error("PostgreSQL standard seeding failed", "error", err)
					os.Exit(1)
				}
				cancel()
				slog.Info("Analyzing users_standard table...")
				_, _ = pool.Exec(ctx, "ANALYZE users_standard")
			}
		case "optimized-postgresql":
			optRepo = dbAdapter.NewOptimizedRepository(pool)
			
			// For optimized partitioned table, target time is BootstrapTime + 2 hours
			targetTime := domain.BootstrapTime.Add(2 * time.Hour)
			partitionName := "cache_opt_partition_" + targetTime.Format("2006_01_02_15")
			
			var count int64
			var tableExists bool
			errExists := pool.QueryRow(ctx, 
				"SELECT EXISTS (SELECT FROM pg_tables WHERE schemaname = 'public' AND tablename = $1)", 
				partitionName,
			).Scan(&tableExists)
			
			if errExists == nil && tableExists {
				err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM "+partitionName).Scan(&count)
			}
			
			if errExists == nil && tableExists && err == nil && count == int64(nbKey) {
				slog.Info("PostgreSQL partition table already fully populated. Skipping seeding.", "partition", partitionName, "count", count)
				slog.Info("Analyzing cache_optimized_partitioned table...")
				_, _ = pool.Exec(ctx, "ANALYZE cache_optimized_partitioned")
			} else {
				slog.Info("PostgreSQL partition table is not populated or has mismatch. Truncating parent and seeding...", "partition", partitionName)
				// Truncate the entire parent table to clear old partitions instantly
				_, _ = pool.Exec(ctx, "TRUNCATE TABLE cache_optimized_partitioned")
				
				slog.Info("Seeding records into cache_optimized_partitioned...")
				seedCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
				if err := optRepo.Seed(seedCtx, dummyUsers, 2*time.Hour); err != nil {
					cancel()
					slog.Error("PostgreSQL optimized seeding failed", "error", err)
					os.Exit(1)
				}
				cancel()
				slog.Info("Analyzing cache_optimized_partitioned table...")
				_, _ = pool.Exec(ctx, "ANALYZE cache_optimized_partitioned")
			}
		}

	default:
		slog.Error("Invalid engine flag specified", "engine", *engine)
		os.Exit(1)
	}

	// 6. Setup HTTP Server and use the ultra-fast direct switch router
	handler := httpAdapter.NewHandler(*engine, globalKeys, naiveRepo, optRepo, stdRepo, valkeyRepo, memoryStore)

	server := &fasthttp.Server{
		Handler:      handler.Handle,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// 7. Graceful Shutdown Setup
	serverCtx, serverStopCtx := context.WithCancel(context.Background())
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	go func() {
		<-sigChan
		slog.Info("Shutdown signal received. Stopping server gracefully...")
		if err := server.Shutdown(); err != nil {
			slog.Error("Graceful shutdown failed", "error", err)
		}
		serverStopCtx()
	}()

	// 8. Garbage Collector & Memory Monitor (Runs at Debug level)
	go func() {
		var m runtime.MemStats
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				runtime.ReadMemStats(&m)
				slog.Debug("GC Monitor stats",
					"alloc_mb", m.Alloc/1024/1024,
					"total_alloc_mb", m.TotalAlloc/1024/1024,
					"sys_mb", m.Sys/1024/1024,
					"num_gc", m.NumGC,
					"last_pause_ns", m.PauseNs[(m.NumGC+255)%256],
				)
			case <-serverCtx.Done():
				return
			}
		}
	}()

	// 9. Global Timestamp Updater Ticker
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				domain.CurrentTimeUTC.Store(time.Now().UTC())
			case <-serverCtx.Done():
				return
			}
		}
	}()

	slog.Info("Bootstrapping complete. Cache benchmark HTTP server listening", "port", port)
	if err := server.ListenAndServe(":" + port); err != nil {
		slog.Error("HTTP server failure", "error", err)
		os.Exit(1)
	}

	<-serverCtx.Done()
	slog.Info("Shutdown complete.")
}

func generateDeterministicULID(i int) [16]byte {
	var ulid [16]byte
	// Constant timestamp: 2026-06-01 12:00:00 UTC (1780324800000 ms)
	ms := int64(1780324800000)

	ulid[0] = byte(ms >> 40)
	ulid[1] = byte(ms >> 32)
	ulid[2] = byte(ms >> 24)
	ulid[3] = byte(ms >> 16)
	ulid[4] = byte(ms >> 8)
	ulid[5] = byte(ms)

	// Entropy: write the index in the last 8 bytes
	ulid[6] = 0xAA
	ulid[7] = 0x55
	ulid[8] = byte(i >> 56)
	ulid[9] = byte(i >> 48)
	ulid[10] = byte(i >> 40)
	ulid[11] = byte(i >> 32)
	ulid[12] = byte(i >> 24)
	ulid[13] = byte(i >> 16)
	ulid[14] = byte(i >> 8)
	ulid[15] = byte(i)

	return ulid
}
