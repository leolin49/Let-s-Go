package gee

import (
	"net/http"
	"strings"
)

type router struct {
	roots		map[string]*Node	
	handlers 	map[string]HandleFunc
}

func newRouter() *router {
	return &router{
		roots: 		make(map[string]*Node),
		handlers: 	make(map[string]HandleFunc),
	}
}

func parsePattern(pattern string) []string {
	vs := strings.Split(pattern, "/")
	parts := make([]string, 0)
	for _, item := range vs {
		if len(item) > 0 {
			parts = append(parts, item)
			if item[0] == '*' {
				break
			}
		}
	}
	return parts
}

func (r *router) addRoute(method string, pattern string, handler HandleFunc) {
	if _, ok := r.roots[method]; !ok {
		// First time to register the method.
		// Build the Trie root for the method.
		r.roots[method] = &Node{}
	}
	parts := parsePattern(pattern)
	// Insert the pattern as a Node to the Trie.
	r.roots[method].insert(pattern, parts, 0)

	// key -> handler
	// e.g. key = GET-v2/hello/:name
	key := method + "-" + pattern
	r.handlers[key] = handler
}

func (r *router) getRoute(method string, path string) (*Node, map[string]string) {
	searchParts := parsePattern(path)
	params := make(map[string]string)

	root, ok := r.roots[method]
	if !ok {
		return nil, nil
	}
	
	n := root.search(searchParts, 0)
	if n == nil {
		return nil, nil
	}
	parts := parsePattern(n.pattern)
	for index, part := range parts {
		if part[0] == ':' {
			// e.g. searchParts = ["hello", "geektutu"]	
			// 		part = ':name'
			// 		params[lang] = go
			params[part[1:]] = searchParts[index]
		} else if part[0] == '*' && len(part) > 1 {
			// e.g. searchParts = ["assets", "css", "geektutu.css"]	
			//		part = '*filepath'
			//		params[filepath] = css/geektutu.css
			params[part[1:]] = strings.Join(searchParts[index:], "/")
			break
		}
	}
	return n, params
}

func (r *router) handle(c *Context) {
	n, params := r.getRoute(c.Method, c.Path)
	if n != nil {
		c.Params = params	
		key := c.Method + "-" + n.pattern
		// Add the route-mapped handler to the end of c.handlers.
		// All middlewares will be executed in order before the user func.
		c.handlers = append(c.handlers, r.handlers[key])
	} else {
		c.handlers = append(c.handlers, func(c *Context){
			c.String(http.StatusNotFound, "404 NOT FOUND: %s\n", c.Path)
		})
	}
	c.Next()
}
