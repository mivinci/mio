package mio

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type Session interface {
	// Addr returns the local address of the underlying transport.
	Addr() net.Addr
	// Open opens a new stream to the other endpoint.
	Open(context.Context) (Stream, error)
	// Accept accepts a new stream from the other endpoint.
	Accept() (Stream, error)
	// Close closes the session.
	Close() error
}

type Stream interface {
	net.Conn
	// CloseWrite half closes the stream indicating that no more frames will
	// be sent to the other endpoint.
	CloseWrite() error
}

type frame struct {
	h Header // frame header
	p []byte // frame payload
}

type session struct {
	sync.RWMutex // protects streams
	options      Options
	lastid       uint32
	rwc          io.ReadWriteCloser // underlying transport
	streams      map[uint32]*stream
	accept       chan *stream
	sendq        chan *frame
	closed       chan struct{}

	local  net.Addr
	remote net.Addr
}

func New(rwc io.ReadWriteCloser, opts ...Option) Session {
	options := Options{
		AcceptQueueLimit: 32,
		AcceptTimeout:    time.Millisecond,
		SendQueueLimit:   128,
		ReadBufferLimit:  128,
		FrameSizeLimit:   1024,
		WindowSizeLimit:  0xffffff,
	}
	for _, o := range opts {
		o(&options)
	}

	var local, remote net.Addr
	if c, ok := rwc.(net.Conn); ok {
		local = c.LocalAddr()
		remote = c.RemoteAddr()
	}

	s := &session{
		options: options,
		rwc:     rwc,
		local:   local,
		remote:  remote,
		streams: make(map[uint32]*stream),
		accept:  make(chan *stream, options.AcceptQueueLimit),
		sendq:   make(chan *frame, options.SendQueueLimit),
		closed:  make(chan struct{}),
	}

	go s.recv()
	go s.send()
	return s
}

func (s *session) recv() {
	for {
		select {
		case <-s.closed:
			return
		default:
		}

		var h Header
		if _, err := io.ReadFull(s.rwc, h[:]); err != nil {
			s.closeWith(ErrTransportClosed)
			return
		}

		if err := s.handleFrame(&h); err != nil {
			s.handleError(h.StreamID(), err)
		}
	}
}

func (s *session) send() {
	for {
		select {
		case <-s.closed:
			return
		case fr := <-s.sendq:
			// any error occurred during writing to the underlying transport
			// closes the entire session with an error ErrInternal
			if _, err := s.rwc.Write(fr.h[:]); err != nil {
				goto CLOSE
			}
			if fr.p != nil {
				if _, err := s.rwc.Write(fr.p); err != nil {
					goto CLOSE
				}
			}
		}
	}
CLOSE:
	s.closeWith(ErrInternal)
}

func (s *session) handleError(id uint32, err error) {
	var code ErrorCode
	var flag Flag

	switch err {
	case ErrFlowControl:
		code = FLowControlError
		flag = RST
	case ErrClosed:
		code = Closed
		flag = RST
	case ErrRefused:
		code = Refused
		flag = RST
	case ErrTransportClosed:
		code = TransportClosed
		flag = Fin
	default:
		code = InternalError
		flag = RST
	}

	h := Header{}
	h.SetStreamID(id)
	h.SetErrorCode(code)
	h.SetFlag(flag)
	fr := &frame{h: h}
	s.sendFrame(fr)
}

func (s *session) sendFrame(fr *frame) error {
	select {
	case <-s.closed:
	case s.sendq <- fr: // TODO: should we timeout here?
	}
	return nil
}

func (s *session) handleFrame(h *Header) error {
	switch flag := h.Flag(); {
	case flag.Isset(Syn):
		s.RLock()
		defer s.RUnlock()
		str, ok := s.streams[h.StreamID()]
		if !ok {
			return ErrClosed
		}
		return str.handleFrame(io.LimitReader(s.rwc, int64(h.Size())), h)
	case flag.Isset(RST):
		s.Lock()
		defer s.Unlock()

		id := h.StreamID()
		if flag.Isset(WND) {
			return s.handleStreamOpen(id, h.Size())
		}

		str, ok := s.streams[id]
		if !ok {
			return ErrClosed
		}
		str.closeWith(fromErrorCode(h.ErrorCode()))
		// unlike TCP, we delete it immediately cuz the sender is required to
		// drop the stream right after sending this frame.
		delete(s.streams, id)
		return nil
	case flag.Isset(WND):
		return s.handleWindowSizeUpdate(h.Size())
	case flag.Isset(Fin):
		return s.closeWith(fromErrorCode(h.ErrorCode()))
	}
	return nil
}

func (s *session) remove(id uint32) {
	s.Lock()
	delete(s.streams, id)
	s.Unlock()
}

func (s *session) Open(ctx context.Context) (Stream, error) {
	select {
	case <-s.closed:
		return nil, ErrTransportClosed
	default:
	}

	id := atomic.AddUint32(&s.lastid, 2)
	if id&(1<<31) > 0 {
		return nil, ErrExhausted
	}

	str := &stream{
		id:    id,
		limit: s.options.FrameSizeLimit,
		win: &window{
			Cond:  sync.Cond{L: &sync.Mutex{}},
			avail: s.options.WindowSizeLimit,
		},
		buf: &buffer{
			Cond:   sync.Cond{L: &sync.Mutex{}},
			Buffer: bytes.Buffer{},
			limit:  s.options.ReadBufferLimit,
		},
		sess: s,
		local: addr{
			id: id,
			tr: s.local,
		},
		remote: addr{
			id: id,
			tr: s.remote,
		},
	}

	s.Lock()
	s.streams[id] = str
	s.Unlock()

	h := Header{}
	h.SetStreamID(id)
	h.SetSize(uint32(s.options.WindowSizeLimit))
	h.SetFlag(flagStreamOpen)
	if err := s.sendFrame(&frame{h: h}); err != nil {
		return nil, err
	}
	return str, nil
}

func (s *session) Accept() (Stream, error) {
	select {
	case <-s.closed:
		return nil, ErrTransportClosed
	case str := <-s.accept:
		return str, nil
	}
}

func (s *session) Close() error {
	select {
	case <-s.closed:
		return nil
	default:
	}

	h := Header{}
	h.SetFlag(Fin)
	return s.sendFrame(&frame{h: h})
}

func (s *session) Addr() net.Addr {
	return s.local
}

func (s *session) handleStreamOpen(id, size uint32) error {
	select {
	case <-s.closed:
		return ErrTransportClosed
	default:
	}

	atomic.StoreUint32(&s.lastid, id)

	str := &stream{
		id:    id,
		limit: s.options.FrameSizeLimit,
		win: &window{
			Cond:  sync.Cond{L: &sync.Mutex{}},
			avail: int(size),
		},
		buf: &buffer{
			Cond:   sync.Cond{L: &sync.Mutex{}},
			Buffer: bytes.Buffer{},
			limit:  s.options.ReadBufferLimit,
		},
		sess: s,
		local: addr{
			id: id,
			tr: s.local,
		},
		remote: addr{
			id: id,
			tr: s.remote,
		},
	}

	s.streams[id] = str

	t := time.NewTicker(s.options.AcceptTimeout)
	defer t.Stop()

	select {
	case <-s.closed:
		return ErrTransportClosed
	case s.accept <- str:
	case <-t.C:
		return ErrRefused
	}
	return nil
}

func (s *session) handleWindowSizeUpdate(size uint32) error {
	return nil
}

func (s *session) closeWith(err error) error {
	select {
	case <-s.closed:
		return nil
	default:
	}

	close(s.closed)

	s.Lock()
	defer s.Unlock()

	streams := make(map[uint32]*stream, len(s.streams))
	for k, v := range s.streams {
		streams[k] = v
	}
	for _, str := range streams {
		str.closeWith(err)
	}
	return nil
}

type stream struct {
	sync.Mutex
	id    uint32
	limit int     // max frame size
	win   *window // flow controller
	buf   *buffer // read buffer
	sess  *session

	local  addr
	remote addr
}

func (s *stream) handleFrame(r io.Reader, h *Header) error {
	flag := h.Flag()
	if flag.Isset(WND) {
		// update window size for the stream
		return s.win.Incr(int(h.Size()))
	}
	if _, err := s.buf.ReadFrom(r); err != nil {
		return err
	}
	if flag.Isset(HFR) {
		// half close the stream meaning no more
		// frames will be sent by the sender
		s.buf.throw(io.EOF)
	}
	return nil
}

func (s *stream) Read(p []byte) (int, error) {
	n, err := s.buf.Read(p)
	if err != nil {
		return 0, err
	}
	// tell the sender to update its window size
	h := Header{}
	h.SetStreamID(s.id)
	h.SetSize(uint32(n))
	h.SetFlag(flagStreamWindowUpdate)
	s.sess.sendFrame(&frame{h: h})
	return n, err
}

func (s *stream) Write(p []byte) (int, error) {
	return s.write(p, false)
}

func (s *stream) CloseWrite() error {
	_, err := s.write(nil, true)
	return err
}

func (s *stream) Close() error {
	h := Header{}
	h.SetStreamID(s.id)
	h.SetErrorCode(NoError)
	h.SetFlag(flagStreamClose)
	s.sess.sendFrame(&frame{h: h})

	// unlike TCP, we don't wait for the receiver to allows us to close
	return s.closeWith(ErrClosed)
}

func (s *stream) LocalAddr() net.Addr {
	return s.local
}

func (s *stream) RemoteAddr() net.Addr {
	return s.remote
}

func (s *stream) SetReadDeadline(t time.Time) error {
	return nil
}

func (s *stream) SetWriteDeadline(t time.Time) error {
	return nil
}

func (s *stream) SetDeadline(t time.Time) error {
	return nil
}

func (s *stream) write(p []byte, last bool) (n int, err error) {
	s.Lock()
	defer s.Unlock()

	h := Header{}
	t := len(p)
	r := t
	nw := 0
	for r > 0 || last {
		min := r
		if min > s.limit {
			min = s.limit
		}

		nw, err = s.win.Decr(min)
		if err != nil {
			return
		}

		m := n + nw
		hc := last && m == t

		h.SetStreamID(s.id)
		h.SetSize(uint32(nw))
		if hc {
			h.SetFlag(flagStreamHalfClose)
		} else {
			h.SetFlag(Syn)
		}

		fr := &frame{h, p[n:m]}
		if err = s.sess.sendFrame(fr); err != nil {
			return
		}

		r -= nw
		n += nw

		if hc {
			s.win.throw(ErrClosed)
			last = false
		}
	}
	return
}

func (s *stream) closeWith(err error) error {
	s.buf.throw(err)
	s.win.throw(err)
	s.sess.remove(s.id)
	return nil
}

type window struct {
	sync.Cond
	avail int
	err   error
}

func (w *window) Incr(n int) error {
	w.L.Lock()
	w.avail += n
	w.Broadcast()
	w.L.Unlock()
	return nil
}

func (w *window) Decr(n int) (int, error) {
	if n <= 0 {
		return 0, nil
	}

	w.L.Lock()
	defer w.L.Unlock()

	for {
		if w.err != nil {
			return 0, w.err
		}

		if w.avail > 0 {
			if n > w.avail {
				w.avail = 0
				n = w.avail
			} else {
				w.avail -= n
			}
			return n, nil
		}
		w.Wait()
	}
}

func (w *window) throw(err error) {
	w.L.Lock()
	w.err = err
	w.Broadcast()
	w.L.Unlock()
}

type buffer struct {
	sync.Cond
	bytes.Buffer
	limit int
	err   error
}

func (b *buffer) Read(p []byte) (int, error) {
	b.L.Lock()
	defer b.L.Unlock()

	for {
		if b.Buffer.Len() > 0 {
			return b.Buffer.Read(p)
		}
		if b.err != nil {
			return 0, b.err
		}
		b.Wait()
	}
}

func (b *buffer) ReadFrom(r io.Reader) (int64, error) {
	b.L.Lock()
	defer b.L.Unlock()

	if b.err != nil {
		// we have to drain all the payload if there's any
		if _, err := io.ReadAll(r); err != nil {
			return 0, ErrInternal
		}
		return 0, ErrClosed
	}

	n, err := b.Buffer.ReadFrom(r)
	if err != nil {
		return 0, ErrInternal
	}

	if int(n) > b.limit {
		b.err = ErrFlowControl
		return 0, ErrFlowControl
	}
	b.Broadcast()
	return n, nil
}

func (b *buffer) throw(err error) {
	b.L.Lock()
	b.err = err
	b.Broadcast()
	b.L.Unlock()
}

type addr struct {
	id uint32
	tr net.Addr // address of the underlying connection
}

func (a addr) Network() string {
	return a.tr.Network() + ":mio"
}

func (a addr) String() string {
	return fmt.Sprintf("%s:%d", a.tr, a.id)
}

type Options struct {
	AcceptQueueLimit int
	AcceptTimeout    time.Duration
	SendQueueLimit   int
	ReadBufferLimit  int
	FrameSizeLimit   int
	WindowSizeLimit  int
}

type Option func(*Options)

func WithAcceptQueueLimit(n int) Option {
	return func(o *Options) {
		o.AcceptQueueLimit = n
	}
}

func WithAcceptTimeout(d time.Duration) Option {
	return func(o *Options) {
		o.AcceptTimeout = d
	}
}

func WithSendQueueLimit(n int) Option {
	return func(o *Options) {
		o.SendQueueLimit = n
	}
}

func WithReadBufferLimit(n int) Option {
	return func(o *Options) {
		o.ReadBufferLimit = n
	}
}

func WithFrameSizeLimit(n int) Option {
	return func(o *Options) {
		o.FrameSizeLimit = n
	}
}

func WithWindowSizeLimit(n int) Option {
	return func(o *Options) {
		o.WindowSizeLimit = n
	}
}
