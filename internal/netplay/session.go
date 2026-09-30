package netplay

import (
	"bufio"
	"fmt"
	"net"
	"sync"
	"time"
)

// Inbound is one message received from the network. From is the connection
// id the host assigned (0 for anything a client receives, -1 unused).
type Inbound struct {
	Msg  Msg
	From int
}

// Host is the authoritative LAN peer: it listens, receives messages from
// clients (delivered to the game loop through Incoming) and broadcasts to
// everyone. All application logic (applying ops, assigning seq, replies)
// runs on the game loop that drains Incoming — the transport never touches
// the document.
//
// The host's OWN messages go through Send, which broadcasts them directly
// (the host's engine already applied them locally).
type Host struct {
	ln        net.Listener
	mu        sync.Mutex
	conns     map[int]net.Conn
	next      int
	in        chan Inbound
	done      chan struct{}
	closeOnce sync.Once
}

// Listen starts hosting on addr (e.g. ":3310"). Returns an error if the
// port is taken.
func Listen(addr string) (*Host, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("netplay listen: %w", err)
	}
	h := &Host{
		ln:    ln,
		conns: map[int]net.Conn{},
		in:    make(chan Inbound, 128),
		done:  make(chan struct{}),
	}
	go h.acceptLoop()
	return h, nil
}

// Addr returns the bound address (useful with port 0 in tests).
func (h *Host) Addr() string { return h.ln.Addr().String() }

// Incoming delivers client messages to the game loop.
func (h *Host) Incoming() <-chan Inbound { return h.in }

// SelfID is the host's participant id.
func (h *Host) SelfID() string { return "host" }

// acceptLoop registers new connections and runs their read loops.
func (h *Host) acceptLoop() {
	for {
		conn, err := h.ln.Accept()
		if err != nil {
			return // listener closed
		}
		h.mu.Lock()
		h.next++
		id := h.next
		h.conns[id] = conn
		h.mu.Unlock()
		go h.readLoop(conn, id)
	}
}

// readLoop decodes messages until the connection dies, then reports a
// leave so the game loop can clean up the peer.
func (h *Host) readLoop(conn net.Conn, id int) {
	defer func() {
		select {
		case h.in <- Inbound{Msg: Msg{Kind: KindLeave}, From: id}:
		case <-h.done:
		}
	}()
	reader := bufio.NewReader(conn)
	for {
		var m Msg
		if err := Decode(reader, &m); err != nil {
			return
		}
		select {
		case h.in <- Inbound{Msg: m, From: id}:
		case <-h.done:
			return
		}
	}
}

// Send broadcasts a message from the host itself to every client.
func (h *Host) Send(m Msg) { h.Broadcast(m) }

// Broadcast sends m to every client. Called from the game loop only, so
// per-connection writes are naturally serialized.
func (h *Host) Broadcast(m Msg) { h.broadcast(m, -1) }

// BroadcastExcept relays m to every client except the connection it came
// from (used when forwarding a client”s op/chat/presence to the others).
func (h *Host) BroadcastExcept(m Msg, exceptConn int) { h.broadcast(m, exceptConn) }

// broadcast sends m to every connection except skip (-1 = none).
func (h *Host) broadcast(m Msg, skip int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, conn := range h.conns {
		if id == skip {
			continue
		}
		_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
		if err := Encode(conn, m); err != nil {
			_ = conn.Close() // dead peer: drop on next read
		}
	}
}

// SendTo replies to one client (welcome messages).
func (h *Host) SendTo(id int, m Msg) {
	h.mu.Lock()
	defer h.mu.Unlock()
	conn, ok := h.conns[id]
	if !ok {
		return
	}
	_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	_ = Encode(conn, m)
}

// Drop closes one client connection (e.g. after a leave).
func (h *Host) Drop(id int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if conn, ok := h.conns[id]; ok {
		_ = conn.Close()
		delete(h.conns, id)
	}
}

// Close shuts the listener and every connection.
func (h *Host) Close() error {
	h.closeOnce.Do(func() {
		close(h.done)
		_ = h.ln.Close()
		h.mu.Lock()
		for _, conn := range h.conns {
			_ = conn.Close()
		}
		h.conns = map[int]net.Conn{}
		h.mu.Unlock()
	})
	return nil
}

// Client is a LAN participant: it dials the host and shuttles messages
// between the game loop (Send / Incoming) and the socket.
type Client struct {
	conn      net.Conn
	id        string
	in        chan Inbound
	out       chan Msg
	done      chan struct{}
	closeOnce sync.Once
	errMu     sync.Mutex
	err       error
}

// Dial connects to a host, registers with name and starts pumping.
// The id is generated locally (crypto randomness is overkill here; a
// timestamp-pid mix suffices for a LAN session).
func Dial(addr, name, id string) (*Client, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("netplay dial: %w", err)
	}
	c := &Client{
		conn: conn,
		id:   id,
		in:   make(chan Inbound, 128),
		out:  make(chan Msg, 256),
		done: make(chan struct{}),
	}
	go c.readLoop()
	go c.writeLoop()
	c.Send(Msg{Kind: KindHello, ID: id, Name: name})
	return c, nil
}

// Incoming delivers host messages to the game loop.
func (c *Client) Incoming() <-chan Inbound { return c.in }

// SelfID returns the client's chosen participant id.
func (c *Client) SelfID() string { return c.id }

// Send queues a message to the host (blocks briefly if the queue is full).
func (c *Client) Send(m Msg) {
	select {
	case c.out <- m:
	case <-c.done:
	}
}

// Closed reports whether the connection has gone away.
func (c *Client) Closed() bool {
	select {
	case <-c.done:
		return true
	default:
		return false
	}
}

// Err returns the read/write error that ended the session (if any).
func (c *Client) Err() error {
	c.errMu.Lock()
	defer c.errMu.Unlock()
	return c.err
}

func (c *Client) setErr(err error) {
	c.errMu.Lock()
	if c.err == nil {
		c.err = err
	}
	c.errMu.Unlock()
}

func (c *Client) readLoop() {
	reader := bufio.NewReader(c.conn)
	for {
		var m Msg
		if err := Decode(reader, &m); err != nil {
			c.setErr(err)
			c.Close()
			return
		}
		select {
		case c.in <- Inbound{Msg: m}:
		case <-c.done:
			return
		}
	}
}

func (c *Client) writeLoop() {
	for {
		select {
		case m := <-c.out:
			_ = c.conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
			if err := Encode(c.conn, m); err != nil {
				c.setErr(err)
				c.Close()
				return
			}
		case <-c.done:
			return
		}
	}
}

// Close tears the connection down (idempotent).
func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		close(c.done)
		_ = c.conn.Close()
	})
	return nil
}

// Session is what the shell needs from either hosting or joining: a
// message pump, a way to send own messages (host = broadcast, client =
// send to host), an identity and teardown.
type Session interface {
	Incoming() <-chan Inbound
	Send(m Msg)
	SelfID() string
	Close() error
}

var (
	_ Session = (*Host)(nil)
	_ Session = (*Client)(nil)
)
