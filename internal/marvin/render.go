package marvin

import (
	"bytes"
	"fmt"
	"text/template"

	"github.com/mooneeb/amgi/internal/event"
)

func RenderTemplates(
	titleTmpl string,
	noteTmpl string,
	e *event.Event,
) (string, string, error) {
	title, err := template.New("title").Parse(titleTmpl)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse title template: %w", err)
	}
	note, err := template.New("note").Parse(noteTmpl)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse note template: %w", err)
	}

	var buf bytes.Buffer
	err = title.Execute(&buf, e)
	if err != nil {
		return "", "", fmt.Errorf("failed to execute title template: %w", err)
	}

	t := buf.String()
	buf.Reset()
	err = note.Execute(&buf, e)
	if err != nil {
		return "", "", fmt.Errorf("failed to execute note template: %w", err)
	}
	n := buf.String()
	return t, n, nil
}
