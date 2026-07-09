package http

import (
	crand "crypto/rand"
	"encoding/binary"
	"math/rand"
	"sync"
	"time"

	"github.com/Chroq/benchmark-caching/adapters/valkey"
	"github.com/Chroq/benchmark-caching/domain"
	"github.com/Chroq/benchmark-caching/ports"
	"github.com/valyala/fasthttp"
)

// Global sync.Pool to recycle UserData structures and eliminate allocation overhead on reads.
var userDataPool = sync.Pool{
	New: func() any {
		return &domain.UserData{}
	},
}

// Handler handles benchmark HTTP requests using fasthttp.
type Handler struct {
	engine      string
	globalKeys  [][16]byte
	prngPool    *sync.Pool
	naiveRepo   ports.NaiveUserRepository
	optRepo     ports.OptimizedUserRepository
	stdRepo     ports.StandardUserRepository
	valkeyRepo  *valkey.Repository
	memoryStore *sync.Map
}

// NewHandler creates a wired HTTP benchmark handler.
func NewHandler(
	engine string,
	globalKeys [][16]byte,
	naiveRepo ports.NaiveUserRepository,
	optRepo ports.OptimizedUserRepository,
	stdRepo ports.StandardUserRepository,
	valkeyRepo *valkey.Repository,
	memoryStore *sync.Map,
) *Handler {
	// Initialize a sync.Pool for non-blocking local PRNGs to avoid global rand mutex contention
	prngPool := &sync.Pool{
		New: func() any {
			var seed [8]byte
			_, _ = crand.Read(seed[:])
			source := rand.NewSource(int64(binary.LittleEndian.Uint64(seed[:])))
			return rand.New(source)
		},
	}

	return &Handler{
		engine:      engine,
		globalKeys:  globalKeys,
		prngPool:    prngPool,
		naiveRepo:   naiveRepo,
		optRepo:     optRepo,
		stdRepo:     stdRepo,
		valkeyRepo:  valkeyRepo,
		memoryStore: memoryStore,
	}
}

// Handle implements the ultra-fast direct switch-based router on top of fasthttp.
func (h *Handler) Handle(ctx *fasthttp.RequestCtx) {
	path := ctx.Path()
	method := ctx.Method()

	switch string(path) {
	case "/health":
		if string(method) == "GET" {
			ctx.SetStatusCode(fasthttp.StatusOK)
			ctx.SetBodyString(`{"status":"OK"}`)
		} else {
			ctx.SetStatusCode(fasthttp.StatusMethodNotAllowed)
		}

	case "/memory/get":
		if h.engine == "memory" {
			if string(method) == "GET" {
				h.HandleMemoryGet(ctx)
			} else {
				ctx.SetStatusCode(fasthttp.StatusMethodNotAllowed)
			}
		} else {
			ctx.SetStatusCode(fasthttp.StatusNotFound)
		}

	case "/memory/set":
		if h.engine == "memory" {
			if string(method) == "POST" {
				h.HandleMemorySet(ctx)
			} else {
				ctx.SetStatusCode(fasthttp.StatusMethodNotAllowed)
			}
		} else {
			ctx.SetStatusCode(fasthttp.StatusNotFound)
		}

	case "/valkey/get":
		if h.engine == "valkey" {
			if string(method) == "GET" {
				h.HandleValkeyGet(ctx)
			} else {
				ctx.SetStatusCode(fasthttp.StatusMethodNotAllowed)
			}
		} else {
			ctx.SetStatusCode(fasthttp.StatusNotFound)
		}

	case "/valkey/set":
		if h.engine == "valkey" {
			if string(method) == "POST" {
				h.HandleValkeySet(ctx)
			} else {
				ctx.SetStatusCode(fasthttp.StatusMethodNotAllowed)
			}
		} else {
			ctx.SetStatusCode(fasthttp.StatusNotFound)
		}

	case "/postgres/get":
		if string(method) == "GET" {
			if h.engine == "naive-postgresql" {
				h.HandlePostgresNaiveGet(ctx)
			} else if h.engine == "optimized-postgresql" {
				h.HandlePostgresOptimizedGet(ctx)
			} else if h.engine == "standard-postgresql" {
				h.HandlePostgresStandardGet(ctx)
			} else {
				ctx.SetStatusCode(fasthttp.StatusNotFound)
			}
		} else {
			ctx.SetStatusCode(fasthttp.StatusMethodNotAllowed)
		}

	case "/postgres/set":
		if string(method) == "POST" {
			if h.engine == "naive-postgresql" {
				h.HandlePostgresNaiveSet(ctx)
			} else if h.engine == "optimized-postgresql" {
				h.HandlePostgresOptimizedSet(ctx)
			} else if h.engine == "standard-postgresql" {
				h.HandlePostgresStandardSet(ctx)
			} else {
				ctx.SetStatusCode(fasthttp.StatusNotFound)
			}
		} else {
			ctx.SetStatusCode(fasthttp.StatusMethodNotAllowed)
		}

	default:
		ctx.SetStatusCode(fasthttp.StatusNotFound)
	}
}

// Helper to get a random key from the pre-populated slice
func (h *Handler) getRandomKey() [16]byte {
	rng := h.prngPool.Get().(*rand.Rand)
	idx := rng.Intn(len(h.globalKeys))
	key := h.globalKeys[idx]
	h.prngPool.Put(rng)
	return key
}

// Helper to generate a dummy UserData payload
func generateDummyUser(id [16]byte) *domain.UserData {
	now := time.Now().Unix()
	return &domain.UserData{
		ID:        id,
		FirstName: "Jean-Sébastien",
		LastName:  "Bach",
		BirthDate: -6468729600,
		Active:    true,
		CreatedAt: now,
		UpdatedAt: now,
		DeletedAt: 0,
	}
}

// ============================================================================
// 1. IN-MEMORY ENDPOINTS (sync.Map)
// ============================================================================

func (h *Handler) HandleMemoryGet(ctx *fasthttp.RequestCtx) {
	key := h.getRandomKey()

	_, ok := h.memoryStore.Load(key)
	if !ok {
		ctx.SetStatusCode(fasthttp.StatusNotFound)
		return
	}

	ctx.SetStatusCode(fasthttp.StatusOK)
}

func (h *Handler) HandleMemorySet(ctx *fasthttp.RequestCtx) {
	key := h.getRandomKey()
	user := generateDummyUser(key)

	h.memoryStore.Store(key, user)
	ctx.SetStatusCode(fasthttp.StatusOK)
}

// ============================================================================
// 2. VALKEY ENDPOINTS (go-redis)
// ============================================================================

func (h *Handler) HandleValkeyGet(ctx *fasthttp.RequestCtx) {
	key := h.getRandomKey()

	user := userDataPool.Get().(*domain.UserData)
	defer func() {
		user.Reset()
		userDataPool.Put(user)
	}()

	found, err := h.valkeyRepo.Get(ctx, key, user)
	if err != nil {
		ctx.SetStatusCode(fasthttp.StatusInternalServerError)
		return
	}

	if !found {
		ctx.SetStatusCode(fasthttp.StatusNotFound)
		return
	}

	ctx.SetStatusCode(fasthttp.StatusOK)
}

func (h *Handler) HandleValkeySet(ctx *fasthttp.RequestCtx) {
	key := h.getRandomKey()
	user := generateDummyUser(key)

	// Valkey SET with strict 5 minutes TTL
	if err := h.valkeyRepo.Set(ctx, user, 5*time.Minute); err != nil {
		ctx.SetStatusCode(fasthttp.StatusInternalServerError)
		return
	}

	ctx.SetStatusCode(fasthttp.StatusOK)
}

// ============================================================================
// 3. POSTGRES NAIVE ENDPOINTS (JSON + VARCHAR key)
// ============================================================================

func (h *Handler) HandlePostgresNaiveGet(ctx *fasthttp.RequestCtx) {
	key := h.getRandomKey()
	keyStr := domain.UUIDToString(key)

	user := userDataPool.Get().(*domain.UserData)
	defer func() {
		user.Reset()
		userDataPool.Put(user)
	}()

	found, err := h.naiveRepo.Get(ctx, keyStr, user)
	if err != nil {
		ctx.SetStatusCode(fasthttp.StatusInternalServerError)
		return
	}

	if !found {
		ctx.SetStatusCode(fasthttp.StatusNotFound)
		return
	}

	ctx.SetStatusCode(fasthttp.StatusOK)
}

func (h *Handler) HandlePostgresNaiveSet(ctx *fasthttp.RequestCtx) {
	key := h.getRandomKey()
	keyStr := domain.UUIDToString(key)
	user := generateDummyUser(key)

	// Postgres set with 5 minutes TTL
	if err := h.naiveRepo.Set(ctx, keyStr, user, 5*time.Minute); err != nil {
		ctx.SetStatusCode(fasthttp.StatusInternalServerError)
		return
	}

	ctx.SetStatusCode(fasthttp.StatusOK)
}

// ============================================================================
// 4. POSTGRES OPTIMIZED ENDPOINTS (Protobuf + ULID key)
// ============================================================================

func (h *Handler) HandlePostgresOptimizedGet(ctx *fasthttp.RequestCtx) {
	key := h.getRandomKey()

	user := userDataPool.Get().(*domain.UserData)
	defer func() {
		user.Reset()
		userDataPool.Put(user)
	}()

	found, err := h.optRepo.Get(ctx, key, user)
	if err != nil {
		ctx.SetStatusCode(fasthttp.StatusInternalServerError)
		return
	}

	if !found {
		ctx.SetStatusCode(fasthttp.StatusNotFound)
		return
	}

	ctx.SetStatusCode(fasthttp.StatusOK)
}

func (h *Handler) HandlePostgresOptimizedSet(ctx *fasthttp.RequestCtx) {
	key := h.getRandomKey()
	user := generateDummyUser(key)

	// Postgres set with 5 minutes TTL
	if err := h.optRepo.Set(ctx, user, 5*time.Minute); err != nil {
		ctx.SetStatusCode(fasthttp.StatusInternalServerError)
		return
	}

	ctx.SetStatusCode(fasthttp.StatusOK)
}

// ============================================================================
// 5. POSTGRES STANDARD RELATIONAL ENDPOINTS (direct SQL mapping)
// ============================================================================

func (h *Handler) HandlePostgresStandardGet(ctx *fasthttp.RequestCtx) {
	key := h.getRandomKey()

	user := userDataPool.Get().(*domain.UserData)
	defer func() {
		user.Reset()
		userDataPool.Put(user)
	}()

	found, err := h.stdRepo.Get(ctx, key, user)
	if err != nil {
		ctx.SetStatusCode(fasthttp.StatusInternalServerError)
		return
	}

	if !found {
		ctx.SetStatusCode(fasthttp.StatusNotFound)
		return
	}

	ctx.SetStatusCode(fasthttp.StatusOK)
}

func (h *Handler) HandlePostgresStandardSet(ctx *fasthttp.RequestCtx) {
	key := h.getRandomKey()
	user := generateDummyUser(key)

	// Postgres standard set with 5 minutes TTL
	if err := h.stdRepo.Set(ctx, user, 5*time.Minute); err != nil {
		ctx.SetStatusCode(fasthttp.StatusInternalServerError)
		return
	}

	ctx.SetStatusCode(fasthttp.StatusOK)
}
