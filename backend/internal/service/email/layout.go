package email

import "strings"

// The branded shell every outbound email is rendered into: dark ground, white
// rounded card, gradient accent bar, logo lockup, separator, then the caller's
// content, then the footer. Table-based layout with all styles inline, because
// Gmail, Outlook and Yahoo strip <style> blocks and ignore most modern CSS.
//
// Splitting this out of HTMLTemplate is what lets a Mail compose arbitrary
// blocks into the content cell without every caller re-deriving email-safe
// markup. The markup itself is unchanged from the original single-purpose
// template.
const (
	shellHead = `<!DOCTYPE html>
<html lang="en" xmlns="http://www.w3.org/1999/xhtml">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<meta http-equiv="X-UA-Compatible" content="IE=edge">
<meta name="color-scheme" content="light dark">
<title>Remote</title>
<!--[if mso]>
<style>table,td{font-family:Arial,Helvetica,sans-serif!important}</style>
<![endif]-->
</head>
<body style="margin:0;padding:0;background-color:#0e0f12;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;-webkit-font-smoothing:antialiased;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="background-color:#0e0f12;">
<tr><td align="center" style="padding:40px 16px;">

<!-- Card -->
<table role="presentation" width="560" cellpadding="0" cellspacing="0" border="0" style="max-width:560px;width:100%;background-color:#ffffff;border-radius:12px;overflow:hidden;">

<!-- Gradient accent bar -->
<tr><td style="height:3px;background:linear-gradient(90deg,#2f6feb,#60c8ff);font-size:0;line-height:0;">&nbsp;</td></tr>

<!-- Logo area -->
<tr><td align="center" style="padding:36px 40px 0 40px;">
<table role="presentation" cellpadding="0" cellspacing="0" border="0"><tr><td align="center">
<!-- Inline SVG logo mark: browser window with cloud -->
<img src="data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSI0OCIgaGVpZ2h0PSI0OCIgdmlld0JveD0iMCAwIDQ4IDQ4Ij48ZGVmcz48bGluZWFyR3JhZGllbnQgaWQ9ImciIHgxPSIwJSIgeTE9IjAlIiB4Mj0iMTAwJSIgeTI9IjEwMCUiPjxzdG9wIG9mZnNldD0iMCUiIHN0b3AtY29sb3I9IiMyZjZmZWIiLz48c3RvcCBvZmZzZXQ9IjEwMCUiIHN0b3AtY29sb3I9IiM2MGM4ZmYiLz48L2xpbmVhckdyYWRpZW50PjwvZGVmcz48cmVjdCB4PSI0IiB5PSI2IiB3aWR0aD0iMzYiIGhlaWdodD0iMzAiIHJ4PSI1IiBmaWxsPSJub25lIiBzdHJva2U9InVybCgjZykiIHN0cm9rZS13aWR0aD0iMy41Ii8+PGNpcmNsZSBjeD0iMTEiIGN5PSIxMi41IiByPSIxLjUiIGZpbGw9InVybCgjZykiLz48Y2lyY2xlIGN4PSIxNiIgY3k9IjEyLjUiIHI9IjEuNSIgZmlsbD0idXJsKCNnKSIvPjxjaXJjbGUgY3g9IjIxIiBjeT0iMTIuNSIgcj0iMS41IiBmaWxsPSJ1cmwoI2cpIi8+PHBhdGggZD0iTTMxIDI4YTggOCAwIDAgMC0xNiAwIiBmaWxsPSJ1cmwoI2cpIiBvcGFjaXR5PSIwLjkiLz48ZWxsaXBzZSBjeD0iMjMiIGN5PSIyNSIgcng9IjEwIiByeT0iNy41IiBmaWxsPSJ1cmwoI2cpIi8+PC9zdmc+" alt="Remote" width="48" height="48" style="display:block;border:0;width:48px;height:48px;">
</td></tr><tr><td align="center" style="padding-top:12px;">
<span style="font-size:22px;font-weight:700;color:#2f6feb;letter-spacing:-0.02em;">Remote</span>
</td></tr></table>
</td></tr>

<!-- Separator -->
<tr><td style="padding:24px 40px 0 40px;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0">
<tr><td style="height:1px;background-color:#e5e7eb;font-size:0;line-height:0;">&nbsp;</td></tr>
</table>
</td></tr>

<!-- Content -->
<tr><td align="center" style="padding:32px 40px 40px 40px;">
`

	shellTail = `
</td></tr>

<!-- Footer -->
<tr><td style="background-color:#f4f5f8;border-radius:0 0 12px 12px;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0">
<tr><td align="center" style="padding:20px 40px;">
<p style="margin:0;font-size:12px;line-height:1.5;color:#8e8f99;">
Sent by Remote &middot; Powered by FutrX
</p>
</td></tr>
</table>
</td></tr>

</table>
<!-- /Card -->

</td></tr>
</table>
</body>
</html>`

	// textFooter is the plain-text counterpart of the HTML footer, appended to
	// every generated text/plain alternative so both parts carry the same
	// attribution.
	textFooter = "Sent by Remote · Powered by FutrX"
)

// renderShell wraps already-rendered, already-escaped content in the branded
// card. Callers pass block HTML, never user input.
func renderShell(content string) string {
	var sb strings.Builder
	sb.Grow(len(shellHead) + len(content) + len(shellTail))
	sb.WriteString(shellHead)
	sb.WriteString(content)
	sb.WriteString(shellTail)
	return sb.String()
}

// blockGap is the vertical space between stacked blocks. It is applied as
// cell padding rather than a margin because Outlook drops margins on block
// elements.
const blockGap = `padding:0 0 20px 0;`

// renderBlocks renders blocks into the HTML card and the plain-text
// alternative in one pass, so the two parts can never drift apart.
func renderBlocks(blocks []block) (htmlBody, textBody string) {
	var h, t strings.Builder
	h.WriteString(`<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0">`)
	for i, b := range blocks {
		style := blockGap
		if i == len(blocks)-1 {
			style = `padding:0;`
		}
		h.WriteString(`<tr><td style="`)
		h.WriteString(style)
		h.WriteString(`">`)
		h.WriteString(b.html())
		h.WriteString(`</td></tr>`)
		if line := b.text(); line != "" {
			if t.Len() > 0 {
				t.WriteString("\n\n")
			}
			t.WriteString(line)
		}
	}
	h.WriteString(`</table>`)
	if t.Len() > 0 {
		t.WriteString("\n\n--\n")
		t.WriteString(textFooter)
	}
	return renderShell(h.String()), t.String()
}
