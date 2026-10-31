// Command tts-cli is a small developer harness around the transfer package,
// used to test send/receive between two local processes without the GUI.
//
//	tts-cli send [-code CODE] [-relay HOST:PORT] [-pass PW] [-zip] PATH...
//	tts-cli recv [-relay HOST:PORT] [-pass PW] [-out DIR] [-decline] CODE
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"

	"github.com/troopers/transfer-service/transfer"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: tts-cli send|recv ...")
		os.Exit(2)
	}
	fs := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	relay := fs.String("relay", "", "relay address (default: public pool)")
	pass := fs.String("pass", "", "relay password")
	code := fs.String("code", "", "custom code phrase (send)")
	zip := fs.Bool("zip", false, "zip folders before sending")
	out := fs.String("out", ".", "destination folder (recv)")
	decline := fs.Bool("decline", false, "decline the offer (recv)")
	noLocal := fs.Bool("nolocal", false, "skip LAN discovery")
	result := fs.String("result", "", "also write ERROR/DONE outcome to this file (for -H windowsgui tests)")
	_ = fs.Parse(os.Args[2:])
	transfer.SetDebug(os.Getenv("TTS_DEBUG") != "")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	settings := transfer.Settings{RelayAddress: *relay, RelayPassword: *pass, ZipFolders: *zip, NoLocal: *noLocal}
	cb := transfer.Callbacks{
		OnCode: func(c string) { fmt.Printf("CODE %s\n", c) },
		OnProgress: func(p transfer.Progress) {
			fmt.Printf("PROGRESS phase=%q file=%q %d/%d bytes=%d/%d speed=%s/s eta=%s\n",
				p.Phase, p.CurrentFile, p.FileIndex, p.FileCount, p.BytesDone, p.BytesTotal,
				transfer.FormatBytes(int64(p.Speed)), p.ETA)
		},
		OnOffer: func(o transfer.Offer) bool {
			fmt.Printf("OFFER %d files, %s\n", len(o.Files), transfer.FormatBytes(o.TotalSize))
			for _, f := range o.Files {
				fmt.Printf("  %s (%s) zipped=%v\n", f.Path, transfer.FormatBytes(f.Size), f.Zipped)
			}
			return !*decline
		},
	}

	var (
		res transfer.Result
		err error
	)
	switch os.Args[1] {
	case "send":
		res, err = transfer.Send(ctx, transfer.SendRequest{Paths: fs.Args(), Code: *code, Settings: settings}, cb)
	case "recv":
		res, err = transfer.Receive(ctx, transfer.ReceiveRequest{Code: fs.Arg(0), Destination: *out, Settings: settings}, cb)
	default:
		err = errors.New("unknown command " + os.Args[1])
	}
	if *result != "" {
		outcome := fmt.Sprintf("DONE %d bytes %v\n", res.Bytes, res.Files)
		if err != nil {
			outcome = "ERROR " + err.Error() + "\n"
		}
		_ = os.WriteFile(*result, []byte(outcome), 0o644)
	}
	if err != nil {
		fmt.Printf("ERROR %s\n", err)
		var fe *transfer.FriendlyError
		if errors.As(err, &fe) && fe.Details() != "" {
			fmt.Printf("DETAILS %s\n", fe.Details())
		}
		os.Exit(1)
	}
	fmt.Printf("DONE %d bytes in %s\n", res.Bytes, res.Duration)
	for _, f := range res.Files {
		fmt.Printf("  %s\n", f)
	}
}
