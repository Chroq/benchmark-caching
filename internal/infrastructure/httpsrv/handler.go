package httpsrv

import (
	crand "crypto/rand"
	"encoding/binary"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/Chroq/benchmark-caching/internal/domain/model"
	"github.com/Chroq/benchmark-caching/internal/domain/port/input"
	googleuuid "github.com/google/uuid"
	oklogulid "github.com/oklog/ulid/v2"
	"github.com/valyala/fasthttp"
)

// Global sync.Pool to recycle UserData structures and eliminate allocation overhead on reads.
var userDataPool = sync.Pool{
	New: func() any {
		return &model.UserData{}
	},
}

// Handler handles benchmark HTTP requests using fasthttp.
type Handler struct {
	engine     string
	globalKeys [][16]byte
	prngPool   *sync.Pool
	useCase    input.UserUseCase
}

// NewHandler creates a wired HTTP benchmark handler using the driving UserUseCase.
func NewHandler(
	engine string,
	globalKeys [][16]byte,
	useCase input.UserUseCase,
) *Handler {
	prngPool := &sync.Pool{
		New: func() any {
			var seed [8]byte
			_, _ = crand.Read(seed[:])
			source := rand.NewSource(int64(binary.LittleEndian.Uint64(seed[:])))
			return rand.New(source)
		},
	}

	return &Handler{
		engine:     engine,
		globalKeys: globalKeys,
		prngPool:   prngPool,
		useCase:    useCase,
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

	case "/memory/get", "/valkey/get", "/postgres/get":
		if string(method) == "GET" {
			h.HandleGet(ctx)
		} else {
			ctx.SetStatusCode(fasthttp.StatusMethodNotAllowed)
		}

	case "/memory/set", "/valkey/set", "/postgres/set":
		if string(method) == "POST" {
			h.HandleSet(ctx)
		} else {
			ctx.SetStatusCode(fasthttp.StatusMethodNotAllowed)
		}

	default:
		ctx.SetStatusCode(fasthttp.StatusNotFound)
	}
}

func (h *Handler) getKeyFromRequest(ctx *fasthttp.RequestCtx) ([16]byte, error) {
	idParam := ctx.QueryArgs().Peek("id")
	if len(idParam) == 26 {
		if parsed, err := oklogulid.Parse(string(idParam)); err == nil {
			return parsed, nil
		}
	} else if len(idParam) == 36 {
		if parsed, err := googleuuid.Parse(string(idParam)); err == nil {
			return parsed, nil
		}
	}
	return [16]byte{}, fmt.Errorf("invalid ID format")
}

func generateDummyUser(id [16]byte) *model.UserData {
	now := time.Now().Unix()
	return &model.UserData{
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

// HandleGet executes the GET use case for the benchmark entry.
func (h *Handler) HandleGet(ctx *fasthttp.RequestCtx) {
	key, err := h.getKeyFromRequest(ctx)
	if err != nil {
		ctx.SetStatusCode(fasthttp.StatusBadRequest)
		return
	}

	user := userDataPool.Get().(*model.UserData)
	defer func() {
		user.Reset()
		userDataPool.Put(user)
	}()

	found, err := h.useCase.GetUser(ctx, key, user)
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

// HandleSet executes the SET use case for the benchmark entry.
func (h *Handler) HandleSet(ctx *fasthttp.RequestCtx) {
	key, err := h.getKeyFromRequest(ctx)
	if err != nil {
		ctx.SetStatusCode(fasthttp.StatusBadRequest)
		return
	}
	user := generateDummyUser(key)

	if err := h.useCase.SetUser(ctx, user, 5*time.Minute); err != nil {
		ctx.SetStatusCode(fasthttp.StatusInternalServerError)
		return
	}

	ctx.SetStatusCode(fasthttp.StatusOK)
}
