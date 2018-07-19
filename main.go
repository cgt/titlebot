package main // import "cgt.name/pkg/titlebot"

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/PuerkitoBio/goquery"
	irc "github.com/fluffle/goirc/client"
)

var (
	ErrUnsupportedContentType = errors.New("unsupported content type")
	ErrNoTitle                = errors.New("empty or no title")
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

	channel := ircURL.Path[1:] // strip '/'
	if channel == "" {
		fmt.Fprintln(os.Stderr, "argument error: missing channel in IRC URL")
		os.Exit(1)
	}
	channel = "#" + channel

	cfg := irc.NewConfig("TitleBot", "titlebot", "TitleBot")
	cfg.Version = "Mozilla/5.0 TitleBot/1.99999993"
	cfg.QuitMessage = ""

	cfg.Server = ircURL.Host
	cfg.SSLConfig = &tls.Config{
		ServerName:         ircURL.Hostname(),
		InsecureSkipVerify: false,
	}

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
		cfg.SSLConfig.InsecureSkipVerify = true
	}

	c := irc.Client(cfg)
	c.EnableStateTracking()

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
	c.HandleFunc(
		irc.JOIN,
		func(conn *irc.Conn, line *irc.Line) {
			if line.Nick == conn.Me().Nick {
				log.Printf("Joined %s", line.Target())
			}
		},
	)
	c.HandleFunc(
		irc.PART,
		func(conn *irc.Conn, line *irc.Line) {
			if line.Nick == conn.Me().Nick {
				log.Printf("Parted %s", line.Target())
			}
		},
	)
	c.HandleFunc(
		irc.KICK,
		func(conn *irc.Conn, line *irc.Line) {
			if line.Args[1] == conn.Me().Nick {
				log.Printf(
					"Kicked from %s by %s: %s",
					line.Args[0], line.Src, line.Args[2],
				)
			}
		},
	)

	c.HandleFunc(irc.PRIVMSG, handlePRIVMSG)

	if err := c.Connect(); err != nil {
		log.Printf("Error connecting to %s: %v", cfg.Server, err)
		return
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case <-quit:
			log.Printf("Received signal to shut down")
			if c.Connected() {
				c.Quit()
			} else {
				return
			}
		case <-disconnected:
			return
		}
	}
}

var reURL = regexp.MustCompile(`(?i)\b(https?://\S*)\b`)

func handlePRIVMSG(conn *irc.Conn, line *irc.Line) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	foundURLs := reURL.FindAllString(line.Text(), -1)
	for _, x := range foundURLs {
		u, err := url.Parse(x)
		if err != nil {
			continue
		}

		t, err := getTitle(ctx, u)
		if err != nil {
			if err != ErrNoTitle && err != ErrUnsupportedContentType {
				log.Print(err)
			}
			continue
		}

		conn.Privmsgf(line.Target(), "%v | %v", t, u.Hostname())
	}
}

func newRequest(ctx context.Context, method, url string) *http.Request {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		panic(err)
	}

	req.Header.Set("User-Agent", "TitleBot/1.0 (+https://cgt.name/pkg/titlebot)")
	req.Header.Set("Accept", "text/html")
	req.Header.Set("Accept-Charset", "utf-8")
	req.Header.Set("Accept-Language", "en")

	return req.WithContext(ctx)
}

func getTitle(ctx context.Context, u *url.URL) (string, error) {
	res, err := http.DefaultClient.Do(newRequest(ctx, "HEAD", u.String()))
	if err != nil {
		return "", err
	}
	res.Body.Close()

	ct := res.Header.Get("Content-Type")
	if ct != "text/html" && !strings.HasPrefix(ct, "text/html;") {
		return "", ErrUnsupportedContentType
	}
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusMethodNotAllowed {
		return "", fmt.Errorf("non-OK status code: %d", res.StatusCode)
	}

	res, err = http.DefaultClient.Do(newRequest(ctx, "GET", u.String()))
	if err != nil {
		return "", err
	}
	// res.Body is closed by NewDocumentFromResponse
	doc, err := goquery.NewDocumentFromResponse(res)
	if err != nil {
		return "", err
	}

	title := doc.Find("title").First().Text()
	title = strings.TrimSpace(title)
	if title == "" {
		return "", ErrNoTitle
	}
	return title, nil
}
