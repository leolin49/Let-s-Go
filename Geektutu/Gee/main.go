package main

import (
	"fmt"
	"log"
	"net/http"
	"text/template"
	"time"

	"github.com/gee"
)

type student struct {
	Name string
	Age  int8
}

func FormatAsDate(t time.Time) string {
	y, m, d := t.Date()
	return fmt.Sprintf("%d-%02d-%02d", y, m, d)
}

func onlyForV2() gee.HandleFunc {
	return func(c *gee.Context) {
		// Start timer
		t := time.Now()
		// if a server error occurred
		// c.Fail(500, "Internal Server Error")
		// Calculate resolution time
		log.Printf("[%d] %s in %v for group v2", c.StatusCode, c.Req.RequestURI, time.Since(t))
	}
}

func main() {
	r := gee.New()
	r.Use(gee.Logger()) // global midlleware
	r.Use(gee.Recovery())
	r.SetFuncMap(template.FuncMap{
		"FormatAsDate": FormatAsDate,
	})
	r.LoadHTMLGlob("./templates/*")
	r.Static("/assets", "./static")

	r.GET("/", func(ctx *gee.Context) {
		ctx.HTML(http.StatusOK, "css.tmpl", nil)
	})

	stu1 := &student{Name: "Geektutu", Age: 20}
	stu2 := &student{Name: "Jack", Age: 25}
	r.GET("/students", func(ctx *gee.Context) {
		ctx.HTML(http.StatusOK, "arr.tmpl", gee.H{
			"title": "gee",
			"stuArr": [2]*student{stu1, stu2},
		})
	})

	r.GET("/date", func(ctx *gee.Context) {
		ctx.HTML(http.StatusOK, "custom_func.tmpl", gee.H{
			"title": "gee",
			"now": time.Date(2025, 1, 9, 12, 49, 49, 0, time.UTC),
		})
	})

	r.GET("/panic", func(ctx *gee.Context) {
		names := []string{"geektutu"}
		ctx.String(http.StatusOK, names[100])
	})

	r.Run(":9999")
}
