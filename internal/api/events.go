package api

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"

	"coldharbour/internal/events"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type hub struct {
	mu    sync.Mutex
	conns map[uuid.UUID]map[*websocket.Conn]struct{}
}

func newHub() *hub {
	return &hub{conns: map[uuid.UUID]map[*websocket.Conn]struct{}{}}
}

func (h *hub) add(tenant uuid.UUID, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	set := h.conns[tenant]
	if set == nil {
		set = map[*websocket.Conn]struct{}{}
		h.conns[tenant] = set
	}
	set[conn] = struct{}{}
}

func (h *hub) remove(tenant uuid.UUID, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	set := h.conns[tenant]
	delete(set, conn)
	if len(set) == 0 {
		delete(h.conns, tenant)
	}
}

func (h *hub) send(tenant uuid.UUID, payload []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for conn := range h.conns[tenant] {
		_ = conn.Write(context.Background(), websocket.MessageText, payload)
	}
}

func (s *Server) StartEvents(ctx context.Context) error {
	return s.hub.listen(ctx, s.rdb)
}

func (h *hub) listen(ctx context.Context, rdb *redis.Client) error {
	sub := rdb.Subscribe(ctx, events.Channel)
	if err := sub.Ping(ctx); err != nil {
		_ = sub.Close()
		return err
	}
	go h.read(ctx, sub)
	return nil
}

func (h *hub) read(ctx context.Context, sub *redis.PubSub) {
	defer sub.Close()
	ch := sub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			var event events.JobEvent
			if err := json.Unmarshal([]byte(msg.Payload), &event); err != nil || event.TenantID == "" {
				continue
			}
			tenant, err := uuid.Parse(event.TenantID)
			if err != nil {
				continue
			}
			h.send(tenant, []byte(msg.Payload))
		}
	}
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	p, _, _, ok := s.authenticate(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if p.Role != "admin" && p.Role != "app" {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	tenant := p.TenantID
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: s.wsOrigins})
	if err != nil {
		return
	}
	s.hub.add(tenant, conn)
	defer s.hub.remove(tenant, conn)
	defer conn.Close(websocket.StatusNormalClosure, "")
	ctx := r.Context()
	for {
		if _, _, err := conn.Read(ctx); err != nil {
			return
		}
	}
}
