package world

import (
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"
)

// Config may be used to create a new World. It holds a variety of fields that
// influence the World.
type Config struct {
	// Log is the Logger that will be used to log errors and debug messages to.
	// If set to nil, slog.Default() is set.
	Log *slog.Logger
	// Dim is the Dimension of the World. If set to nil, the World will use
	// Overworld as its dimension. The dimension set here influences, among
	// others, the sky colour, weather/time and liquid behaviour in that World.
	Dim Dimension
	// PortalDestination is a function that returns the destination World for a
	// portal of a specific Dimension type. If set to nil, no portals will
	// function. If the function returns a nil world for a Dimension, only
	// portals of that specific Dimension type will not function.
	PortalDestination func(dim Dimension) *World
	// Provider is the Provider implementation used to read and write World
	// data. If set to nil, the Provider used will be NopProvider, which does
	// not store any data to disk.
	Provider Provider
	// Generator is the Generator implementation used to generate new areas of
	// the World. If set to nil, the Generator used will be NopGenerator, which
	// generates completely empty chunks.
	Generator Generator
	// ReadOnly specifies if the World should be read-only, meaning no new data
	// will be written to the Provider.
	ReadOnly bool
	// SaveInterval specifies how often a World should be automatically saved to
	// disk. This includes chunks, entities and level.dat data. If ReadOnly is
	// set to false, changing SaveInterval will have no effect.
	// By default, SaveInterval is set to 10 minutes. Setting SaveInterval to
	// a negative number disables automatic saving entirely.
	SaveInterval time.Duration
	// RandomTickSpeed specifies the rate at which blocks should be ticked in
	// the World. By default, each sub chunk has 3 blocks randomly ticked per
	// sub chunk, so the default value is 3. Setting this value to -1 or lower
	// will stop random ticking altogether, while setting it higher results in
	// faster ticking.
	RandomTickSpeed int
	// RandSource is the rand.Source used for generation of random numbers in a
	// World, such as when selecting blocks to tick or when deciding where to
	// strike lightning. If set to nil, RandSource defaults to a `rand.PCG`
	// source seeded with `time.Now().UnixNano()`. PCG is significantly faster
	// than `rand.ChaCha8` on 64-bit systems at the expense of poorer
	// statistical distribution, which is acceptable here.
	// See https://go.dev/blog/chacha8rand.
	RandSource rand.Source
	// Entities is an EntityRegistry with all Entity types registered that may
	// be added to the World.
	Entities EntityRegistry
}

// New creates a new World using the Config conf. The World returned will start
// ticking as soon as a viewer is added to it and is otherwise ready for use.
func (conf Config) New() *World {
	if conf.Log == nil {
		conf.Log = slog.Default()
	}
	if conf.Dim == nil {
		conf.Dim = Overworld
	}
	if conf.Provider == nil {
		conf.Provider = NopProvider{}
	}
	if conf.SaveInterval == 0 {
		conf.SaveInterval = time.Minute * 10
	}
	if conf.Generator == nil {
		conf.Generator = NopGenerator{}
	}
	if conf.RandomTickSpeed == 0 {
		conf.RandomTickSpeed = 3
	}
	if conf.RandSource == nil {
		t := uint64(time.Now().UnixNano())
		conf.RandSource = rand.NewPCG(t, t)
	}
	s := conf.Provider.Settings()
	w := &World{
		scheduledUpdates: newScheduledTickQueue(s.CurrentTick),
		entities:         make(map[*EntityHandle]ChunkPos),
		viewers:          make(map[*Loader]Viewer),
		chunks:           make(map[ChunkPos]*Column),
		closing:          make(chan struct{}),
		queue:            make(chan transaction, 128),
		r:                rand.New(conf.RandSource),
		advance:          s.ref.Add(1) == 1,
		conf:             conf,
		ra:               conf.Dim.Range(),
		set:              s,
	}
	w.weather = weather{w: w}
	var h Handler = NopHandler{}
	w.handler.Store(&h)

	w.running.Add(4)

	t := ticker{interval: time.Second / 20}
	go t.tickLoop(w)
	go w.autoSave()
	go w.handleTransactions()

	go func() {
		t2 := time.NewTicker(time.Second)
		defer t2.Stop()
		for {
			select {
			case <-t2.C:
				// Detect deadlock
				w.runningTxMu.Lock()
				if w.runningTx != nil && time.Since(w.runningTxAt) > 5*time.Second {
					panicMsg := "Deadlock detected in world transaction. The transaction has been running for more than 5 seconds."
					panicMsg += "\nPENDING TRANSACTIONS:"

					txs := w.queue

					close(w.queue)

					no := 0
					for pendingTx := range txs {
						no++
						if no > 10 {
							panicMsg += "\n..."
							break
						}
						panicMsg += "\n\n" + fmt.Sprintf("TX NO %d:", no)
						panicMsg += "\n------------------ BEGIN stack trace ------------------"
						for _, trace := range pendingTx.CallerInfo() {
							panicMsg += "\n" + trace
						}
						panicMsg += "\n------------------- END stack trace -------------------"
					}

					panicMsg += "\n\nHANGING TRANSACTION:"
					panicMsg += "\n------------------ BEGIN stack trace ------------------"
					for _, trace := range w.runningTx.CallerInfo() {
						panicMsg += "\n" + trace
					}
					panicMsg += "\n------------------- END stack trace -------------------"

					panic(panicMsg)
				}
				w.runningTxMu.Unlock()
			case <-w.closing:
				w.running.Done()
				return
			}
		}
	}()

	<-w.Exec(t.tick)
	return w
}
