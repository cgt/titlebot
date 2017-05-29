package main // import "cgt.name/pkg/titlebot"

import (
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"syscall"

	irc "github.com/fluffle/goirc/client"
)

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: %s [options] <IRC URL>\n\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "Example:\n  %s -noverify ircs://mynick:password123@irc.example.net/mychannel\n\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "Options:\n")
	flag.PrintDefaults()
}

func main() {
	flag.Usage = usage

	flagInsecureSkipVerify := flag.Bool(
		"noverify",
		false,
		"do not verify server's TLS certificate chain and host name",
	)
	flag.Parse()

	if flag.NArg() < 1 || flag.Arg(0) == "" {
		flag.Usage()
		os.Exit(1)
	}

	ircURL, err := url.Parse(flag.Arg(0))
	if err != nil {
		// Seems url.Parse accepts pretty much any input.
		// Not sure what input would cause this case to run.
		fmt.Fprintf(os.Stderr, "argument error: unable to parse argument as URL: %v\n", err)
		os.Exit(1)
	}

	if ircURL.Scheme != "irc" && ircURL.Scheme != "ircs" {
		fmt.Fprintln(os.Stderr, "argument error: IRC URL scheme must be 'irc' or 'ircs'")
		os.Exit(1)
	}

	channel := ircURL.Path
	if channel == "" {
		fmt.Fprintln(os.Stderr, "argument error: missing channel in IRC URL")
		os.Exit(1)
	}

	cfg := irc.NewConfig("TitleBot", "titlebot", "TitleBot")
	cfg.Version = "Mozilla/5.0 TitleBot/1.99999993"
	cfg.QuitMessage = ""

	cfg.Server = ircURL.Host

	if u := ircURL.User; u != nil {
		if u.Username() != "" {
			cfg.Me.Nick = u.Username()
		}
		if pw, ok := u.Password(); ok {
			cfg.Pass = pw
		}
	}

	if ircURL.Scheme == "ircs" {
		cfg.SSL = true
	}

	if *flagInsecureSkipVerify {
		cfg.SSLConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
	}

	c := irc.Client(cfg)

	c.HandleFunc(
		irc.CONNECTED,
		func(conn *irc.Conn, line *irc.Line) {
			log.Printf("Connected to %s", cfg.Server)
			conn.Join(channel)
		},
	)

	disconnected := make(chan struct{})
	c.HandleFunc(
		irc.DISCONNECTED,
		func(conn *irc.Conn, line *irc.Line) {
			log.Printf("Disconnected from %s", cfg.Server)
			disconnected <- struct{}{}
		},
	)

	c.HandleFunc(irc.PRIVMSG, handlePRIVMSG)

	if err := c.Connect(); err != nil {
		log.Printf("Cannot connect to %s: %v", cfg.Server, err)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case <-quit:
			c.Quit()
		case <-disconnected:
			return
		}
	}
}

var reURL = regexp.MustCompile(`\b(https?://\S*)\b`)

func handlePRIVMSG(conn *irc.Conn, line *irc.Line) {
	foundURLs := reURL.FindAllString(line.Text(), -1)
	for _, x := range foundURLs {
		u, err := url.Parse(x)
		if err != nil {
			continue
		}

		t, err := title(u.String())
		if err != nil {
			log.Print(err)
			continue
		}
		if t == "" {
			continue
		}

		conn.Privmsgf(line.Target(), "%v | %v", t, u)
	}
}
