package crack

import (
	"context"
	"io"
	"net"
	"sync"
)

// tcpRelay exposes a loopback TCP listener that forwards every accepted
// connection to target through dial. It lets protocols whose libraries do not
// accept a custom dialer still honor the configured egress proxy.
type tcpRelay struct {
	listener net.Listener
	target   string
	dial     DialContextFunc
	ctx      context.Context
	cancel   context.CancelFunc

	closeOnce  sync.Once
	wg         sync.WaitGroup
	acceptDone chan struct{}
	mu         sync.Mutex
	conns      map[net.Conn]struct{}
}

func newTCPRelay(ctx context.Context, target string, dial DialContextFunc) (*tcpRelay, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	relayCtx, cancel := context.WithCancel(ctx)
	relay := &tcpRelay{
		listener:   listener,
		target:     target,
		dial:       dial,
		ctx:        relayCtx,
		cancel:     cancel,
		acceptDone: make(chan struct{}),
		conns:      make(map[net.Conn]struct{}),
	}
	go relay.acceptLoop()
	go func() {
		<-relayCtx.Done()
		relay.Close()
	}()
	return relay, nil
}

func (r *tcpRelay) Address() string {
	if r == nil || r.listener == nil {
		return ""
	}
	return r.listener.Addr().String()
}

func (r *tcpRelay) Close() {
	if r == nil {
		return
	}
	r.closeOnce.Do(func() {
		r.cancel()
		_ = r.listener.Close()
		<-r.acceptDone
		r.mu.Lock()
		for conn := range r.conns {
			_ = conn.Close()
		}
		r.mu.Unlock()
		r.wg.Wait()
	})
}

func (r *tcpRelay) acceptLoop() {
	defer close(r.acceptDone)
	for {
		client, err := r.listener.Accept()
		if err != nil {
			return
		}
		r.wg.Add(1)
		go func() {
			defer r.wg.Done()
			r.handle(client)
		}()
	}
}

func (r *tcpRelay) handle(client net.Conn) {
	upstream, err := r.dial(r.ctx, "tcp", r.target)
	if err != nil {
		_ = client.Close()
		return
	}
	clientTracked := r.track(client)
	upstreamTracked := r.track(upstream)
	if !clientTracked || !upstreamTracked {
		if clientTracked {
			r.untrack(client)
		}
		if upstreamTracked {
			r.untrack(upstream)
		}
		_ = client.Close()
		_ = upstream.Close()
		return
	}
	defer r.untrack(client)
	defer r.untrack(upstream)

	var copies sync.WaitGroup
	copies.Add(2)
	go func() {
		defer copies.Done()
		_, _ = io.Copy(upstream, client)
		closeWrite(upstream)
	}()
	go func() {
		defer copies.Done()
		_, _ = io.Copy(client, upstream)
		closeWrite(client)
	}()
	copies.Wait()
	_ = client.Close()
	_ = upstream.Close()
}

func (r *tcpRelay) track(conn net.Conn) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ctx.Err() != nil {
		_ = conn.Close()
		return false
	}
	r.conns[conn] = struct{}{}
	return true
}

func (r *tcpRelay) untrack(conn net.Conn) {
	r.mu.Lock()
	delete(r.conns, conn)
	r.mu.Unlock()
}

func closeWrite(conn net.Conn) {
	if writer, ok := conn.(interface{ CloseWrite() error }); ok {
		_ = writer.CloseWrite()
	}
}
