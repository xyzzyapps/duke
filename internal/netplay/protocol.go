// Package netplay implements the LAN multiplayer layer: a newline-
// delimited JSON protocol over TCP, an authoritative host, and clients
// (including the test bot).
//
// Topology: one peer runs `shooter -serve :3310` (the host owns the
// authoritative buffer); others run `shooter -join host:3310`. Buffer
// edits flow as DocOps: a peer's own engine performs the edit locally
// (for animation) and the ObservedDoc wrapper ships the op to the host,
// which replays it and broadcasts a sequenced copy to everyone else.
// Concurrent edits degrade to last-writer-wins on clamp (documented v1
// conflict model — no OT/CRDT).
//
// Everything in this package is pure Go (net + encoding/json): no
// Ebitengine imports, so it is fully testable — including a real
// localhost integration test.
package netplay

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"shooter/internal/doc"
)

// Message kinds.
const (
	KindHello    = "hello"    // client -> host: introduction
	KindWelcome  = "welcome"  // host -> client: id + full snapshot
	KindOp       = "op"       // buffer mutation (either direction)
	KindPresence = "presence" // participant position/pose (either)
	KindChat     = "chat"     // chat line (either)
	KindLeave    = "leave"    // goodbye
)

// DocOp is a serializable buffer mutation. Kind selects the fields:
//
//	insert   - At + Text
//	delete   - At..End
//	moveline - At.Line + Delta
//	load     - Text (file replaced, e.g. host Ctrl+O)
type DocOp struct {
	Kind  string  `json:"kind"`
	At    doc.Pos `json:"at"`
	End   doc.Pos `json:"end"`
	Text  string  `json:"text,omitempty"`
	Delta int     `json:"delta,omitempty"`
}

// Peer describes one participant for presence and welcome messages.
type Peer struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Line   int    `json:"line,omitempty"`
	Col    int    `json:"col,omitempty"`
	State  string `json:"state,omitempty"`
	Facing int    `json:"facing,omitempty"`
	Chat   string `json:"chat,omitempty"`
}

// Msg is the one wire message (newline-delimited JSON; unused fields are
// omitted). Origin is stamped by the sender and preserved by relays so
// echo loops can be recognised.
type Msg struct {
	Kind string `json:"kind"`

	// hello / welcome
	ID    string `json:"id,omitempty"`
	Name  string `json:"name,omitempty"`
	Text  string `json:"text,omitempty"`  // welcome: full buffer snapshot
	Peers []Peer `json:"peers,omitempty"` // welcome: existing participants

	// op
	Op     *DocOp `json:"op,omitempty"`
	Seq    int    `json:"seq,omitempty"`
	Origin string `json:"origin,omitempty"`

	// presence / chat
	Peer Peer `json:"peer,omitempty"`
}

// Encode writes one message as a single JSON line.
func Encode(w io.Writer, m Msg) error {
	b, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("netplay encode: %w", err)
	}
	b = append(b, '\n')
	_, err = w.Write(b)
	return err
}

// Decode reads exactly one JSON line from a persistent buffered reader.
// The reader MUST be reused across calls for a given stream: reading a
// whole line first (instead of json.Decoder) guarantees no bytes are
// swallowed into a throw-away decoder buffer, so back-to-back messages
// survive.
func Decode(r *bufio.Reader, m *Msg) error {
	line, err := r.ReadString('\n')
	if err != nil {
		return err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return fmt.Errorf("netplay decode: empty line")
	}
	return json.Unmarshal([]byte(line), m)
}

// OpFor builds the DocOp for a plain insert (used by ObservedDoc).
func OpForInsert(p doc.Pos, text string) DocOp {
	return DocOp{Kind: "insert", At: p, Text: text}
}

// OpForDelete builds the DocOp for a range deletion.
func OpForDelete(from, to doc.Pos) DocOp {
	return DocOp{Kind: "delete", At: from, End: to}
}

// OpForMove builds the DocOp for a one-line drag.
func OpForMove(line, delta int) DocOp {
	return DocOp{Kind: "moveline", At: doc.Pos{Line: line}, Delta: delta}
}

// OpForLoad builds the DocOp for a full buffer replacement.
func OpForLoad(text string) DocOp {
	return DocOp{Kind: "load", Text: text}
}
