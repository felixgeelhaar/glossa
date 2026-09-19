package glossa

import "fmt"

// Template functions for html/template and text/template, for email, PDF
// and CLI output. The maps have the untyped map[string]any type, so they
// are assignable to both packages' FuncMap.
//
//	{{t "email.welcome.subject" "name" .Name}}   message with name/value pairs
//	{{t "email.welcome.body" .Args}}             message with an Args map
//	{{td "email.footer" "Thanks!" "name" .Name}} message with an inline default
//	<html lang="{{lang}}" dir="{{dir}}">         active locale and direction
//
// Parse templates once with TemplateFuncs, then bind a Localizer per
// execution on a clone: html/template can only clone templates that were
// never executed, so keep the parsed template as an unexecuted base.
//
//	base := template.Must(template.New("").Funcs(glossa.TemplateFuncs()).ParseFS(emails, "*.html"))
//	tmpl := template.Must(base.Clone())
//	err := tmpl.Funcs(client.For(user.Locale).FuncMap()).ExecuteTemplate(w, "welcome.html", data)

// FuncMap returns the template functions bound to l.
func (l *Localizer) FuncMap() map[string]any {
	return map[string]any{
		"t": func(id string, args ...any) string {
			return l.T(id, l.templateArgs(id, args))
		},
		"td": func(id, defaultText string, args ...any) string {
			return l.T(id, l.templateArgs(id, args), Default(defaultText))
		},
		"lang": l.Locale,
		"dir":  func() string { return string(l.Direction()) },
	}
}

// TemplateFuncs returns placeholders for the template functions, to parse
// templates before a Localizer exists. They render message IDs (or the
// inline default), no locale and "ltr".
func TemplateFuncs() map[string]any {
	return map[string]any{
		"t":    func(id string, _ ...any) string { return id },
		"td":   func(_, defaultText string, _ ...any) string { return defaultText },
		"lang": func() string { return "" },
		"dir":  func() string { return string(LTR) },
	}
}

// templateArgs turns template arguments into Args: a single Args (or
// map[string]any), or name/value pairs. Malformed pairs are reported and
// skipped, so a template bug degrades one placeholder, not the email.
func (l *Localizer) templateArgs(id string, args []any) Args {
	if len(args) == 1 {
		if m, ok := args[0].(map[string]any); ok {
			return m
		}
	}
	out := make(Args, len(args)/2)
	for i := 0; i < len(args); i += 2 {
		name, ok := args[i].(string)
		if !ok || i+1 == len(args) {
			l.reportTemplateMisuse(id, fmt.Sprintf("template argument %d: want name/value pairs, got %T", i, args[i]))
			continue
		}
		out[name] = args[i+1]
	}
	return out
}

func (l *Localizer) reportTemplateMisuse(id, detail string) {
	e := Error{Type: ErrorFormat, Detail: detail, MessageID: id, Locale: l.Locale()}
	if ref, ok := l.c.Release(); ok {
		e.ReleaseID = ref.ID
	}
	l.c.reporter.report(e)
}
