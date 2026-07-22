package httpsrv

import (
	crand "crypto/rand"
	"encoding/binary"
	"errors"
	"math/rand"
	"sync"
	"time"

	"github.com/Chroq/benchmark-caching/internal/domain/model"
	"github.com/Chroq/benchmark-caching/internal/domain/port/input"
	googleuuid "github.com/google/uuid"
	oklogulid "github.com/oklog/ulid/v2"
	"github.com/valyala/fasthttp"
)

var errInvalidID = errors.New("invalid ID format")

// Global sync.Pool to recycle UserData structures and eliminate allocation overhead on reads/writes.
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
	switch string(ctx.Path()) {
	case "/health":
		if ctx.IsGet() {
			ctx.SetStatusCode(fasthttp.StatusOK)
			ctx.SetBodyString(`{"status":"OK"}`)
		} else {
			ctx.SetStatusCode(fasthttp.StatusMethodNotAllowed)
		}

	case "/memory/get", "/valkey/get", "/postgres/get":
		if ctx.IsGet() {
			h.HandleGet(ctx)
		} else {
			ctx.SetStatusCode(fasthttp.StatusMethodNotAllowed)
		}

	case "/memory/set", "/valkey/set", "/postgres/set":
		if ctx.IsPost() {
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
		var parsed oklogulid.ULID
		if err := parsed.UnmarshalText(idParam); err == nil {
			return parsed, nil
		}
	} else if len(idParam) == 36 {
		if parsed, err := googleuuid.ParseBytes(idParam); err == nil {
			return parsed, nil
		}
	}
	return [16]byte{}, errInvalidID
}

func populateDummyUser(id [16]byte, user *model.UserData) {
	now := time.Now().Unix()
	user.ID = id
	user.FirstName = "Jean-Sébastien"
	user.LastName = "Bach"
	user.BirthDate = -6468729600
	user.Active = true
	user.CreatedAt = now
	user.UpdatedAt = now
	user.DeletedAt = 0
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

	user := userDataPool.Get().(*model.UserData)
	defer func() {
		user.Reset()
		userDataPool.Put(user)
	}()

	populateDummyUser(key, user)

	if err := h.useCase.SetUser(ctx, user, 8*time.Hour); err != nil {
		ctx.SetStatusCode(fasthttp.StatusInternalServerError)
		return
	}

	ctx.SetStatusCode(fasthttp.StatusOK)
}
