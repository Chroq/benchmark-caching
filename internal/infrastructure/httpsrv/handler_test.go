package httpsrv_test

import (
	"context"
	"net"
	"testing"

	"github.com/Chroq/benchmark-caching/internal/domain/model"
	"github.com/Chroq/benchmark-caching/internal/domain/service"
	"github.com/Chroq/benchmark-caching/internal/infrastructure/httpsrv"
	"github.com/Chroq/benchmark-caching/internal/infrastructure/memory"
	googleuuid "github.com/google/uuid"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/fasthttputil"
)

func TestHandler_MemoryFlow(t *testing.T) {
	cache := memory.NewCache()
	repo := memory.NewRepository(cache)
	defer repo.Close()

	useCase := service.NewUserService(repo)
	defer useCase.Close()

	keyID := [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	uuidStr := googleuuid.UUID(keyID).String()

	handler := httpsrv.NewHandler("memory", [][16]byte{keyID}, useCase)

	ln := fasthttputil.NewInmemoryListener()
	defer ln.Close()

	go fasthttp.Serve(ln, handler.Handle)

	client := &fasthttp.Client{
		Dial: func(addr string) (net.Conn, error) {
			return ln.Dial()
		},
	}

	// 1. Check /health
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	req.SetRequestURI("http://localhost/health")
	req.Header.SetMethod("GET")
	if err := client.Do(req, resp); err != nil {
		t.Fatalf("Health request failed: %v", err)
	}
	if resp.StatusCode() != fasthttp.StatusOK {
		t.Errorf("expected health 200, got %d", resp.StatusCode())
	}

	// 2. SET item
	req.Reset()
	resp.Reset()
	req.SetRequestURI("http://localhost/memory/set?id=" + uuidStr)
	req.Header.SetMethod("POST")
	if err := client.Do(req, resp); err != nil {
		t.Fatalf("Set request failed: %v", err)
	}
	if resp.StatusCode() != fasthttp.StatusOK {
		t.Errorf("expected set 200, got %d", resp.StatusCode())
	}

	// 3. GET item
	req.Reset()
	resp.Reset()
	req.SetRequestURI("http://localhost/memory/get?id=" + uuidStr)
	req.Header.SetMethod("GET")
	if err := client.Do(req, resp); err != nil {
		t.Fatalf("Get request failed: %v", err)
	}
	if resp.StatusCode() != fasthttp.StatusOK {
		t.Errorf("expected get 200, got %d", resp.StatusCode())
	}

	// 4. Verify item content
	var fetched model.UserData
	found, err := useCase.GetUser(context.Background(), keyID, &fetched)
	if err != nil || !found {
		t.Fatalf("failed to retrieve stored item: %v", err)
	}
	if fetched.FirstName != "Jean-Sébastien" {
		t.Errorf("expected FirstName Jean-Sébastien, got %s", fetched.FirstName)
	}
}

