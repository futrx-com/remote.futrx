package email

// HTMLTemplate renders a branded HTML email with the given heading and body
// text. It is the simplest possible composition - one heading, one paragraph -
// and exists for callers that want exactly that without reaching for the Mail
// builder. Heading and body are HTML-escaped by the blocks they become.
//
// Prefer Mailer.Mail() for anything richer: it renders the same shell and
// produces the matching text/plain alternative at the same time, which this
// function cannot.
func HTMLTemplate(heading, body string) string {
	htmlBody, _ := renderBlocks([]block{
		headingBlock{value: heading},
		textBlock{value: body},
	})
	return htmlBody
}
