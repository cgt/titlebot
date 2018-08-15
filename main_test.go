package main

import (
	"net/url"
	"testing"
)

func TestChannelFromURL_GivenPathWithoutHashPrefix_PrependHash(t *testing.T) {
	ircURL, err := url.Parse("irc://example.com/foo")
	if err != nil {
		panic(err)
	}

	channel := channelFromURL(ircURL)

	if expected := "#foo"; channel != expected {
		t.Errorf("expected %q, got %q", expected, channel)
	}
}

func TestChannelFromURL_GivenPathWithHashPrefix_DoNotPrependHash(t *testing.T) {
	ircURL, err := url.Parse("irc://example.com/%23foo") // %23 is a URL encoded #.
	if err != nil {
		panic(err)
	}

	channel := channelFromURL(ircURL)

	if expected := "#foo"; channel != expected {
		t.Errorf("expected %q, got %q", expected, channel)
	}
}
func TestChannelFromURL_GivenPathIsSingleSlash_ReturnEmptyString(t *testing.T) {
	ircURL, err := url.Parse("irc://example.com/")
	if err != nil {
		panic(err)
	}

	channel := channelFromURL(ircURL)

	if channel != "" {
		t.Errorf("expected empty string, got %q", channel)
	}
}
