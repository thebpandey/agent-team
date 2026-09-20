package dashboard

import (
	"html"
	"strings"
)

func renderSnapshot(s Snapshot) ([]byte, error) {
	var out strings.Builder
	out.Grow(1024)
	out.WriteString("<!doctype html><html lang=\"en\"><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width,initial-scale=1\"><title>Agent Team Dashboard</title><style>body{font:16px system-ui,sans-serif;margin:2rem;max-width:72rem}section{margin:2rem 0}ul{padding-left:1.3rem}li{overflow-wrap:anywhere}table{border-collapse:collapse;width:100%}td,th{border:1px solid #bbb;padding:.4rem;text-align:left;vertical-align:top}</style></head><body>")
	tag(&out, "h1", "Agent Team Dashboard")
	tag(&out, "p", s.Project+" · run "+s.RunID+" · revision "+s.CanonicalRevision)
	tag(&out, "p", "Generated "+s.GeneratedAt+" · "+string(s.Status))
	out.WriteString("<section><h2>Integrations</h2>")
	tag(&out, "p", s.LastIntegration)
	out.WriteString("</section>")
	out.WriteString("<section><h2>Tasks</h2><table><thead><tr><th>ID</th><th>State</th><th>Next</th></tr></thead><tbody>")
	for _, task := range s.Tasks {
		out.WriteString("<tr><td>")
		escaped(&out, task.ID)
		out.WriteString("</td><td>")
		escaped(&out, task.State)
		out.WriteString("</td><td>")
		escaped(&out, task.Next)
		out.WriteString("</td></tr>")
	}
	out.WriteString("</tbody></table></section><section><h2>Teams</h2>")
	for _, team := range s.Teams {
		tag(&out, "h3", team.ID+" · "+team.State)
		tag(&out, "p", "Queue: "+team.QueueFingerprint)
		list(&out, team.Tasks)
	}
	out.WriteString("</section><section><h2>Resources</h2>")
	listSection(&out, "Servers", s.Resources.Servers)
	listSection(&out, "Browsers", s.Resources.Browsers)
	listSection(&out, "External", s.Resources.External)
	out.WriteString("</section><section><h2>Evidence</h2>")
	list(&out, s.Evidence)
	out.WriteString("</section></body></html>")
	return []byte(out.String()), nil
}

func tag(out *strings.Builder, name, value string) {
	out.WriteByte('<')
	out.WriteString(name)
	out.WriteByte('>')
	escaped(out, value)
	out.WriteString("</")
	out.WriteString(name)
	out.WriteByte('>')
}
func escaped(out *strings.Builder, value string) { out.WriteString(html.EscapeString(value)) }
func listSection(out *strings.Builder, name string, values []string) {
	tag(out, "h3", name)
	list(out, values)
}
func list(out *strings.Builder, values []string) {
	out.WriteString("<ul>")
	for _, value := range values {
		tag(out, "li", value)
	}
	out.WriteString("</ul>")
}
