package gee

import (
	"log"
	"net/http"
	"path"
	"strings"
	"text/template"
)

type HandleFunc func(*Context)

// Engine should implement the interface of ServeHTTP.
type Engine struct {
	*RouterGroup
	router 			*router
	groups 			[]*RouterGroup		// store all groups.
	htmlTemplates 	*template.Template 	// Load all the templates into mem.
	funcMap			template.FuncMap	// All the render function.
}

type RouterGroup struct {
	prefix		string
	middlewares []HandleFunc	// support middleware
	parent		*RouterGroup	// support nesting
	engine 		*Engine			// all groups share a Engine instance.
}

// New is the constructor of gee.Engine.
func New() *Engine {
	engine := &Engine{
		router: newRouter(),
	}
	engine.RouterGroup = &RouterGroup{
		engine: engine,
	}
	engine.groups = []*RouterGroup{engine.RouterGroup}
	return engine
}

// Group is defined to create a new RouterGroup.
// remember all groups share the same Engine instance.
func (g *RouterGroup) Group(prefix string) *RouterGroup {
	engine := g.engine
	newGroup := &RouterGroup{
		prefix: g.prefix + prefix,
		parent: g,
		engine: engine,
	}
	engine.groups = append(engine.groups, newGroup)
	return newGroup
}

func (g *RouterGroup) Use(middlewares ...HandleFunc) {
	g.middlewares = append(g.middlewares, middlewares...)
}

func (g *RouterGroup) addRoute(method string, comp string, handler HandleFunc) {
	pattern := g.prefix + comp
	log.Printf("Route %4s - %s", method, pattern)
	g.engine.router.addRoute(method, pattern, handler)
}

// Create static handler.
func (g *RouterGroup) createStaticHandler(relativePath string, fs http.FileSystem) HandleFunc {
	absolutePath := path.Join(g.prefix, relativePath)
	// To serve a directory on disk under an alternate URL path, 
	// use StripPrefix to modify the request URL's path 
	// before the FileServer sees it.
	fileServer := http.StripPrefix(absolutePath, http.FileServer(fs))
	return func(c *Context) {
		file := c.Param("filepath")	
		// Check if file exists and/or if we have permission to access it.
		if _, err := fs.Open(file); err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		fileServer.ServeHTTP(c.Writer, c.Req)
	}
}

// Add GET route.
func (g *RouterGroup) GET(pattern string, handler HandleFunc) {
	g.addRoute("GET", pattern, handler)
}

// Add POST route.
func (g *RouterGroup) POST(pattern string, handler HandleFunc) {
	g.addRoute("POST", pattern, handler)
}

// Static serve static files.
// relativePath: relative uri path in RouterGroup g.
// root: absolute/relative dir path on http server.
func (g *RouterGroup) Static(relativePath string, root string) {
	handler := g.createStaticHandler(relativePath, http.Dir(root))
	urlPattern := path.Join(relativePath, "/*filepath")
	// Register GET handler.
	g.GET(urlPattern, handler)
}

func (e *Engine) SetFuncMap(funcMap template.FuncMap) {
	e.funcMap = funcMap
}

func (e *Engine) LoadHTMLGlob(pattern string) {
	e.htmlTemplates = template.Must(template.New("").Funcs(e.funcMap).ParseGlob(pattern))
}

// Run the http server in addr.
func (e *Engine) Run(addr string) (err error) {
	return http.ListenAndServe(addr, e)
}

// ListenAndServe will call the ServeHTTP method.
func (e *Engine) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// When receive a request, determine which middleware should work.
	var middlewares []HandleFunc
	for _, group := range e.groups {
		if strings.HasPrefix(req.URL.Path, group.prefix) {
			middlewares = append(middlewares, group.middlewares...)
		}
	}
	c := newContext(w, req)
	// Firstly add all the middlewares to the c.handlers.
	c.handlers = append(c.handlers, middlewares...)
	c.engine = e
	// Finally add the router match handler to the c.handlers.
	e.router.handle(c)
}

