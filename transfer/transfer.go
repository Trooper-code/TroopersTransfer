// Package transfer wraps the croc library with a callback-based API suitable
// for a GUI: progress updates, an accept/decline hook for incoming files, and
// plain-English errors.
//
// croc keeps some state process-wide (working directory, stdin prompt reader),
// so only one transfer may run at a time. Send and Receive return ErrBusy if
// another is in progress.
package transfer

import (
	"context"
	"fmt"
	"net"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/schollz/croc/v11/src/codephrase"
	"github.com/schollz/croc/v11/src/croc"
	"github.com/schollz/croc/v11/src/models"
	"github.com/schollz/croc/v11/src/publicrelay"
	"github.com/schollz/croc/v11/src/tcp"
)

// Phase describes where a transfer is in its lifecycle.
type Phase int

const (
	PhasePreparing Phase = iota
	PhaseWaitingForPeer
	PhaseAwaitingApproval
	PhaseTransferring
	PhaseFinishing
)

func (p Phase) String() string {
	switch p {
	case PhasePreparing:
		return "Preparing…"
	case PhaseWaitingForPeer:
		return "Waiting for the other side…"
	case PhaseAwaitingApproval:
		return "Waiting for approval…"
	case PhaseTransferring:
		return "Transferring…"
	case PhaseFinishing:
		return "Finishing…"
	}
	return ""
}

// Progress is a snapshot reported periodically while a transfer runs.
type Progress struct {
	Phase       Phase
	CurrentFile string
	FileIndex   int // 1-based; 0 before the first file starts
	FileCount   int
	BytesDone   int64
	BytesTotal  int64
	Speed       float64       // bytes per second, smoothed
	ETA         time.Duration // 0 when unknown
}

// FileEntry is one item in an incoming offer.
type FileEntry struct {
	Path   string // relative path as it will appear in the destination
	Size   int64
	Zipped bool // a folder the sender zipped; it is unpacked on arrival
}

// Offer is what the sender wants to give us, shown before anything is written.
type Offer struct {
	Files        []FileEntry
	EmptyFolders []string
	TotalSize    int64
}

// Settings that apply to both directions.
type Settings struct {
	RelayAddress  string // empty = croc's public relay pool
	RelayPassword string // empty = croc's default
	ZipFolders    bool   // sender only
	NoLocal       bool   // skip LAN discovery and always go through the relay
}

type SendRequest struct {
	Paths []string
	Code  string // optional custom code phrase
	Settings
}

type ReceiveRequest struct {
	Code        string
	Destination string
	Settings
}

type Callbacks struct {
	// OnCode is called once the sender's code phrase is known.
	OnCode func(code string)
	// OnProgress is called roughly four times a second.
	OnProgress func(Progress)
	// OnOffer is called on the receiving side before any file is written. It
	// blocks the transfer until it returns; true accepts.
	OnOffer func(Offer) bool
}

// Result describes a finished transfer.
type Result struct {
	Code     string
	Files    []string // sender: names sent; receiver: final saved paths
	Bytes    int64
	Duration time.Duration
}

var active sync.Mutex

// Send offers the given files and folders and blocks until the receiver has
// them, the transfer fails, or ctx is canceled.
func Send(ctx context.Context, req SendRequest, cb Callbacks) (res Result, err error) {
	defer func() { err = friendly(err, true) }()
	if len(req.Paths) == 0 {
		return res, &FriendlyError{Message: "Choose at least one file or folder to send."}
	}
	if !active.TryLock() {
		return res, ErrBusy
	}
	defer active.Unlock()
	started := time.Now()

	abs := make([]string, 0, len(req.Paths))
	for _, p := range req.Paths {
		a, err := filepath.Abs(p)
		if err != nil {
			return res, err
		}
		if _, err := os.Stat(a); err != nil {
			return res, &FriendlyError{Message: "Can't open " + filepath.Base(p) + ". It may have been moved or deleted.", Err: err}
		}
		abs = append(abs, a)
	}

	opts := baseOptions(req.Settings)
	opts.IsSender = true
	opts.HashAlgorithm = "xxhash"
	opts.ZipFolder = req.ZipFolders
	opts.SharedSecret = req.Code
	opts.RelayPorts = relayPorts(opts.RelayAddress)

	if err := chooseRelayForSend(ctx, &opts, req.RelayAddress == ""); err != nil {
		return res, err
	}
	res.Code = opts.SharedSecret

	// croc writes zipped folders into the working directory, so give it a
	// private scratch folder for the duration of the send.
	scratch, err := os.MkdirTemp("", "troopers-send-")
	if err != nil {
		return res, err
	}
	defer os.RemoveAll(scratch)
	restore, err := chdir(scratch)
	if err != nil {
		return res, err
	}
	defer restore()

	report(cb, Progress{Phase: PhasePreparing})
	files, emptyFolders, numFolders, err := croc.GetFilesInfoContext(ctx, abs, opts.ZipFolder, false, nil)
	if err != nil {
		return res, err
	}
	var total int64
	for _, f := range files {
		total += f.Size
		res.Files = append(res.Files, path.Join(f.FolderRemote, f.Name))
	}

	userCtx := ctx
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var stalled atomic.Bool

	client, err := croc.NewCtx(ctx, opts)
	if err != nil {
		return res, err
	}
	if cb.OnCode != nil {
		cb.OnCode(opts.SharedSecret)
	}

	stop := watch(ctx, client, cb, watchConfig{
		isSender: true,
		files:    func() []croc.FileInfo { return files },
		phase:    func() Phase { return PhaseWaitingForPeer },
		onStall:  func() { stalled.Store(true); cancel() },
	})
	err = client.Send(files, emptyFolders, numFolders)
	stop()
	switch {
	case stalled.Load():
		return res, errConnectionLost
	case userCtx.Err() != nil:
		return res, ErrCanceled
	}
	if err != nil {
		return res, err
	}
	res.Bytes = total
	res.Duration = time.Since(started)
	return res, nil
}

// Receive connects with the given code, asks OnOffer before writing anything,
// and saves accepted files into req.Destination without overwriting.
func Receive(ctx context.Context, req ReceiveRequest, cb Callbacks) (res Result, err error) {
	defer func() { err = friendly(err, false) }()
	req.Code = NormalizeCode(req.Code)
	if len(req.Code) < 6 {
		return res, &FriendlyError{Message: "Enter the code phrase from the sender (at least 6 characters)."}
	}
	if !active.TryLock() {
		return res, ErrBusy
	}
	defer active.Unlock()
	started := time.Now()
	res.Code = req.Code

	dest, err := filepath.Abs(req.Destination)
	if err != nil {
		return res, err
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return res, &FriendlyError{Message: "Couldn't create the destination folder. Choose a different folder.", Err: err}
	}

	opts := baseOptions(req.Settings)
	opts.SharedSecret = req.Code
	opts.Rename = true
	if req.RelayAddress == "" {
		if err := assignPublicRelayForCode(&opts); err != nil {
			return res, err
		}
	}

	// Everything lands in an empty hidden staging folder first. Only after the
	// transfer succeeds and our own path checks pass is it moved into dest,
	// renaming anything that would collide.
	staging, err := os.MkdirTemp(dest, ".troopers-incoming-")
	if err != nil {
		return res, &FriendlyError{Message: "Couldn't write to the destination folder. Choose a different folder.", Err: err}
	}
	defer os.RemoveAll(staging)
	if !within(dest, staging) {
		return res, errOutsideDestination
	}
	restore, err := chdir(staging)
	if err != nil {
		return res, err
	}
	restoreOnce := sync.OnceFunc(restore)
	defer restoreOnce()

	// With the public relay pool a mistyped code maps to a different relay
	// and room, where croc would wait forever. Give up if no sender shows up.
	userCtx := ctx
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var noSender atomic.Bool

	client, err := croc.NewCtx(ctx, opts)
	if err != nil {
		return res, err
	}

	var (
		mu        sync.Mutex
		phase     = PhaseWaitingForPeer
		offered   []croc.FileInfo
		declined  bool
		unsafeErr error
	)
	timer := time.AfterFunc(NoSenderTimeout, func() {
		if !client.Step1ChannelSecured && getPhaseLocked(&mu, &phase) == PhaseWaitingForPeer {
			noSender.Store(true)
			cancel()
		}
	})
	defer timer.Stop()
	setPhase := func(p Phase) { mu.Lock(); phase = p; mu.Unlock() }
	getPhase := func() Phase { mu.Lock(); defer mu.Unlock(); return phase }

	pctx, cancelPrompt := context.WithCancel(ctx)
	defer cancelPrompt()
	installPrompt(&promptReader{ctx: pctx, ask: func() bool {
		// Runs on croc's goroutine right after it stored the sender's file
		// list, so reading these fields here does not race.
		offer, files, err := buildOffer(client)
		mu.Lock()
		offered = files
		mu.Unlock()
		if err != nil {
			unsafeErr = err
			return false
		}
		setPhase(PhaseAwaitingApproval)
		ok := cb.OnOffer == nil || cb.OnOffer(offer)
		if ok {
			setPhase(PhaseTransferring)
		} else {
			declined = true
		}
		return ok
	}})

	var stalled atomic.Bool
	stop := watch(ctx, client, cb, watchConfig{
		files: func() []croc.FileInfo {
			mu.Lock()
			defer mu.Unlock()
			return offered
		},
		phase:   getPhase,
		onStall: func() { stalled.Store(true); cancel() },
	})
	err = client.Receive()
	stop()
	cancelPrompt()

	switch {
	case stalled.Load():
		return res, errConnectionLost
	case noSender.Load():
		return res, &FriendlyError{Message: "Wrong code, or the sender isn't sending right now. Check the code and make sure the sender has started first."}
	case userCtx.Err() != nil:
		return res, ErrCanceled
	case unsafeErr != nil:
		return res, unsafeErr
	case declined:
		return res, ErrDeclined
	case err != nil:
		return res, err
	}

	report(cb, Progress{Phase: PhaseFinishing})
	restoreOnce()
	if err := verifyTree(staging); err != nil {
		return res, err
	}
	saved, err := moveIntoPlace(staging, dest)
	res.Files = saved
	if err != nil {
		return res, err
	}
	for _, f := range offered {
		res.Bytes += f.Size
	}
	res.Duration = time.Since(started)
	return res, nil
}

// NoSenderTimeout is how long a receiver waits for a sender to appear.
var NoSenderTimeout = 30 * time.Second

func getPhaseLocked(mu *sync.Mutex, p *Phase) Phase {
	mu.Lock()
	defer mu.Unlock()
	return *p
}

// NormalizeCode tidies a pasted code phrase: trims it and turns runs of
// spaces into single hyphens ("curry salsa blunt" -> "curry-salsa-blunt").
func NormalizeCode(code string) string {
	return strings.Join(strings.Fields(code), "-")
}

func buildOffer(c *croc.Client) (Offer, []croc.FileInfo, error) {
	files := append([]croc.FileInfo(nil), c.FilesToTransfer...)
	var offer Offer
	for _, f := range files {
		rel := path.Join(f.FolderRemote, f.Name)
		if _, err := safeRelative(rel); err != nil {
			return offer, files, err
		}
		if f.Symlink != "" {
			target := path.Join(f.FolderRemote, f.Symlink)
			if path.IsAbs(f.Symlink) {
				target = f.Symlink
			}
			if _, err := safeRelative(target); err != nil {
				return offer, files, err
			}
		}
		offer.Files = append(offer.Files, FileEntry{Path: rel, Size: f.Size, Zipped: f.TempFile})
		offer.TotalSize += f.Size
	}
	for _, f := range c.EmptyFoldersToTransfer {
		if _, err := safeRelative(f.FolderRemote); err != nil {
			return offer, files, err
		}
		offer.EmptyFolders = append(offer.EmptyFolders, f.FolderRemote)
	}
	return offer, files, nil
}

func baseOptions(s Settings) croc.Options {
	o := croc.Options{
		Curve:            "p256",
		RelayAddress:     models.DEFAULT_RELAY,
		RelayAddress6:    models.DEFAULT_RELAY6,
		RelayPassword:    models.DEFAULT_PASSPHRASE,
		IgnoreStdin:      true,
		DisableClipboard: true,
		Transport:        croc.TransportAuto,
	}
	if s.RelayAddress != "" {
		o.RelayAddress = s.RelayAddress
		o.RelayAddress6 = ""
	}
	if s.RelayPassword != "" {
		o.RelayPassword = s.RelayPassword
	}
	o.DisableLocal = s.NoLocal
	return o
}

// relayPorts mirrors the croc CLI: a control port plus four transfer ports,
// starting at the relay address's port.
func relayPorts(relay string) []string {
	base := 9009
	if _, p, err := net.SplitHostPort(relay); err == nil {
		if n, err := strconv.Atoi(p); err == nil && n > 0 && n < 65531 {
			base = n
		}
	}
	ports := make([]string, 5)
	for i := range ports {
		ports[i] = strconv.Itoa(base + i)
	}
	return ports
}

// chooseRelayForSend generates a code if needed and, for the public pool,
// picks the relay the code maps to (the receiver derives the same one).
func chooseRelayForSend(ctx context.Context, o *croc.Options, public bool) error {
	if !public {
		if o.SharedSecret == "" {
			code, err := codephrase.Generate()
			if err != nil {
				return err
			}
			o.SharedSecret = code
		}
		return nil
	}
	if o.SharedSecret == "" {
		relays := publicrelay.Relays()
		idx, _, err := publicrelay.SelectFirst(ctx, relays, publicrelay.ProbeTimeout, tcp.MeasureServerLatencyContext)
		if err != nil {
			idx = 0
		}
		code, err := codephrase.GenerateForRelay(idx, len(relays))
		if err != nil {
			return err
		}
		o.SharedSecret = code
	}
	if len(o.SharedSecret) < 6 {
		return &FriendlyError{Message: "The code phrase must be at least 6 characters."}
	}
	return assignPublicRelayForCode(o)
}

func assignPublicRelayForCode(o *croc.Options) error {
	relays := publicrelay.Relays()
	idx, err := codephrase.RelayIndex(o.SharedSecret, len(relays))
	if err != nil {
		return err
	}
	o.RelayAddress = relays[idx]
	o.RelayAddress6 = ""
	o.PublicRelay = true
	return nil
}

func chdir(dir string) (restore func(), err error) {
	prev, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	if err := os.Chdir(dir); err != nil {
		return nil, err
	}
	return func() { _ = os.Chdir(prev) }, nil
}

func report(cb Callbacks, p Progress) {
	if cb.OnProgress != nil {
		cb.OnProgress(p)
	}
}

// StallTimeout is how long a transfer may make no progress before we give up
// on croc's automatic reconnects and report the connection as lost.
var StallTimeout = 45 * time.Second

var errConnectionLost = &FriendlyError{Message: "Connection lost, try again."}

type watchConfig struct {
	isSender bool
	files    func() []croc.FileInfo
	phase    func() Phase
	onStall  func()
}

// watch polls croc's exported counters and reports progress until stop is
// called. croc updates these from its own goroutines without a lock; we only
// read word-sized values, so the worst case is a slightly stale number.
func watch(ctx context.Context, c *croc.Client, cb Callbacks, cfg watchConfig) (stop func()) {
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		tick := time.NewTicker(250 * time.Millisecond)
		defer tick.Stop()
		var (
			shown      int64 // never goes backwards, even when croc resumes
			lastBytes  int64
			lastTime   = time.Now()
			speed      float64
			lastRaw    int64 = -1
			lastChange       = time.Now()
		)
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-tick.C:
			}
			list := cfg.files()
			p := Progress{Phase: cfg.phase(), FileCount: len(list)}
			for _, f := range list {
				p.BytesTotal += f.Size
			}
			if cfg.isSender {
				switch {
				case c.Step3RecipientRequestFile:
					p.Phase = PhaseTransferring
				case c.Step2FileInfoTransferred:
					p.Phase = PhaseAwaitingApproval
				}
			}
			if p.Phase == PhaseTransferring && len(list) > 0 {
				cur := min(max(c.FilesToTransferCurrentNum, 0), len(list)-1)
				var raw int64
				for _, f := range list[:cur] {
					raw += f.Size
				}
				raw += min(c.TotalSent, list[cur].Size)
				raw = min(raw, p.BytesTotal)

				now := time.Now()
				if raw != lastRaw {
					lastRaw, lastChange = raw, now
				} else if raw < p.BytesTotal && now.Sub(lastChange) > StallTimeout && cfg.onStall != nil {
					cfg.onStall()
					return
				}

				shown = max(shown, raw)
				p.BytesDone = shown
				p.FileIndex = cur + 1
				p.CurrentFile = path.Join(list[cur].FolderRemote, list[cur].Name)

				if dt := now.Sub(lastTime).Seconds(); dt > 0 {
					inst := float64(shown-lastBytes) / dt
					if speed == 0 {
						speed = inst
					} else {
						speed = 0.8*speed + 0.2*inst
					}
				}
				lastBytes, lastTime = shown, now
				p.Speed = speed
				if speed > 1 {
					p.ETA = time.Duration(float64(p.BytesTotal-p.BytesDone) / speed * float64(time.Second)).Round(time.Second)
				}
			}
			report(cb, p)
		}
	}()
	return func() {
		close(done)
		wg.Wait()
	}
}

// FormatBytes renders a byte count like "12.3 MB".
func FormatBytes(n int64) string {
	const unit = 1000
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "kMGTPE"[exp])
}

func init() { croc.Debug(false) }

// SetDebug turns croc's verbose logging (to stderr) on or off.
func SetDebug(on bool) { croc.Debug(on) }
