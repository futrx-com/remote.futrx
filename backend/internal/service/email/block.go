package email

import (
	"fmt"
	"html"
	"net/url"
	"strings"
)

// block is one piece of email content. Every block renders twice - once as
// email-safe HTML for the branded card, once as plain text - so the
// text/plain alternative is generated from the same source as the HTML and
// the two can never drift apart. Nothing outside this package implements it:
// the Mail builder is the only way to add blocks.
type block interface {
	html() string
	text() string
}

// esc is the single escaping point for every value a caller hands a block.
// It matches what HTMLTemplate has always done with its heading and body.
func esc(s string) string { return html.EscapeString(s) }

// headingBlock is the title line of the email, centred under the logo.
type headingBlock struct{ value string }

func (b headingBlock) html() string {
	return `<h1 style="margin:0;font-size:24px;font-weight:700;color:#1f242c;line-height:1.3;text-align:center;">` +
		esc(b.value) + `</h1>`
}

func (b headingBlock) text() string { return b.value }

// textBlock is a paragraph of body copy.
type textBlock struct{ value string }

func (b textBlock) html() string {
	return `<p style="margin:0;font-size:15px;line-height:1.6;color:#414852;text-align:left;">` +
		esc(b.value) + `</p>`
}

func (b textBlock) text() string { return b.value }

// buttonBlock is a call to action. It is rendered as a padded table cell
// rather than a styled <a> because Outlook ignores padding on inline
// elements; the link text is repeated in the plain-text part so a client that
// refuses to render HTML still shows the URL.
type buttonBlock struct{ label, target string }

func (b buttonBlock) html() string {
	return `<table role="presentation" cellpadding="0" cellspacing="0" border="0" align="center" style="margin:0 auto;">` +
		`<tr><td align="center" style="background-color:#2f6feb;border-radius:8px;">` +
		`<a href="` + esc(b.target) + `" style="display:inline-block;padding:12px 28px;font-size:15px;font-weight:600;color:#ffffff;text-decoration:none;">` +
		esc(b.label) + `</a></td></tr></table>`
}

func (b buttonBlock) text() string { return b.label + ": " + b.target }

// listBlock is a bulleted list.
type listBlock struct{ items []string }

func (b listBlock) html() string {
	var sb strings.Builder
	sb.WriteString(`<ul style="margin:0;padding:0 0 0 20px;font-size:15px;line-height:1.6;color:#414852;text-align:left;">`)
	for _, item := range b.items {
		sb.WriteString(`<li style="margin:0 0 6px 0;">`)
		sb.WriteString(esc(item))
		sb.WriteString(`</li>`)
	}
	sb.WriteString(`</ul>`)
	return sb.String()
}

func (b listBlock) text() string {
	lines := make([]string, 0, len(b.items))
	for _, item := range b.items {
		lines = append(lines, "- "+item)
	}
	return strings.Join(lines, "\n")
}

// kvBlock is a label/value summary table - run details, schedule times, the
// facts behind a notification.
type kvBlock struct{ pairs [][2]string }

func (b kvBlock) html() string {
	var sb strings.Builder
	sb.WriteString(`<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="font-size:14px;line-height:1.5;text-align:left;">`)
	for _, pair := range b.pairs {
		sb.WriteString(`<tr><td style="padding:0 12px 6px 0;color:#8e8f99;white-space:nowrap;vertical-align:top;">`)
		sb.WriteString(esc(pair[0]))
		sb.WriteString(`</td><td style="padding:0 0 6px 0;color:#1f242c;vertical-align:top;">`)
		sb.WriteString(esc(pair[1]))
		sb.WriteString(`</td></tr>`)
	}
	sb.WriteString(`</table>`)
	return sb.String()
}

func (b kvBlock) text() string {
	lines := make([]string, 0, len(b.pairs))
	for _, pair := range b.pairs {
		lines = append(lines, pair[0]+": "+pair[1])
	}
	return strings.Join(lines, "\n")
}

// codeBlock displays a value meant to be read and copied - a one-time code, a
// token, a command - in a monospace box.
type codeBlock struct{ value string }

func (b codeBlock) html() string {
	return `<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0">` +
		`<tr><td align="center" style="padding:16px;background-color:#f4f5f8;border-radius:8px;` +
		`font-family:'SFMono-Regular',Consolas,'Liberation Mono',Menlo,monospace;font-size:20px;` +
		`font-weight:600;letter-spacing:0.08em;color:#1f242c;word-break:break-all;">` +
		esc(b.value) + `</td></tr></table>`
}

func (b codeBlock) text() string { return b.value }

// dividerBlock is a horizontal rule between sections. It carries no text, so
// it contributes nothing to the plain-text alternative.
type dividerBlock struct{}

func (dividerBlock) html() string {
	return `<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0">` +
		`<tr><td style="height:1px;background-color:#e5e7eb;font-size:0;line-height:0;">&nbsp;</td></tr></table>`
}

func (dividerBlock) text() string { return "" }

// noteBlock is small muted print - expiry warnings, "you can ignore this if
// it wasn't you".
type noteBlock struct{ value string }

func (b noteBlock) html() string {
	return `<p style="margin:0;font-size:13px;line-height:1.5;color:#8e8f99;text-align:left;">` +
		esc(b.value) + `</p>`
}

func (b noteBlock) text() string { return b.value }

// safeURL rejects anything that is not an absolute http(s) URL, so a caller
// cannot smuggle a javascript: or data: target into a button that recipients
// are being invited to click.
func safeURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("%w: button URL is empty", ErrIncompleteMail)
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("%w: button URL is not a URL: %v", ErrIncompleteMail, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("%w: button URL scheme %q is not http or https", ErrIncompleteMail, parsed.Scheme)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("%w: button URL has no host", ErrIncompleteMail)
	}
	return trimmed, nil
}
