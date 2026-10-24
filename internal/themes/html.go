package themes

import (
	"bytes"
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// PrepareHTML namespaces static assets while keeping absolute API URLs and
// navigation links intact. Themes should build with base: './' for lazy chunks.
func PrepareHTML(source []byte, short string) ([]byte, error) {
	doc, err := html.Parse(bytes.NewReader(source))
	if err != nil { return nil, err }
	base := AssetPath(short, "") + "/"
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			for i := range n.Attr {
				a := &n.Attr[i]
				asset := a.Key == "src" || a.Key == "poster" || (a.Key == "href" && n.Data == "link")
				if !asset || a.Val == "" || strings.HasPrefix(a.Val, "//") { continue }
				u, err := url.Parse(a.Val)
				if err != nil || u.IsAbs() || u.Host != "" || strings.HasPrefix(u.Path, "/api/") || strings.HasPrefix(u.Path, "/themes/") { continue }
				u.Path = base + strings.TrimPrefix(u.Path, "/")
				u.RawPath = ""
				a.Val = u.String()
			}
			if n.Data == "head" {
				// An explicit base is necessary for relative imports on SPA deep links.
				for child := n.FirstChild; child != nil; {
					next := child.NextSibling
					if child.Type == html.ElementNode && child.Data == "base" { n.RemoveChild(child) }
					child = next
				}
				n.InsertBefore(&html.Node{Type:html.ElementNode, Data:"base", Attr:[]html.Attribute{{Key:"href", Val:base}}}, n.FirstChild)
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling { walk(child) }
	}
	walk(doc)
	var out bytes.Buffer
	if err := html.Render(&out, doc); err != nil { return nil, err }
	return out.Bytes(), nil
}
