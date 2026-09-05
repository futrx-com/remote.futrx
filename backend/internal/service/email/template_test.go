package email

import (
	"strings"
	"testing"
)

func TestHTMLTemplateContainsContent(t *testing.T) {
	out := HTMLTemplate("Hello World", "This is the body text.")

	if !strings.Contains(out, "Hello World") {
		t.Error("output missing heading")
	}
	if !strings.Contains(out, "This is the body text.") {
		t.Error("output missing body")
	}
}

func TestHTMLTemplateStructure(t *testing.T) {
	out := HTMLTemplate("Test", "Body")

	checks := []string{
		"<!DOCTYPE html>",
		"<table",
		"Remote",
		"Sent by Remote",
		"Powered by FutrX",
		"#0e0f12", // dark background
		"#2f6feb", // accent blue
		"#f4f5f8", // footer bg
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("output missing expected substring %q", want)
		}
	}
}

func TestHTMLTemplateEscapesHTML(t *testing.T) {
	out := HTMLTemplate("<script>alert('xss')</script>", "a<b>c&d")

	if strings.Contains(out, "<script>") {
		t.Error("heading was not HTML-escaped")
	}
	if !strings.Contains(out, "&lt;script&gt;") {
		t.Error("heading should contain escaped script tag")
	}
	if strings.Contains(out, "a<b>c") {
		t.Error("body was not HTML-escaped")
	}
	if !strings.Contains(out, "a&lt;b&gt;c&amp;d") {
		t.Error("body should contain escaped content")
	}
}
