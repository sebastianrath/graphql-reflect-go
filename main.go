package main

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
)

type Cat struct {
	Name  string `json:"name"`
	Age   int    `json:"age"`
	Color string `json:"color"`
}

type Dog struct {
	Name   string `json:"name"`
	Age    int    `json:"age"`
	Color  string `json:"color"`
	Friend Cat    `json:"friend"`

	// Functions can be used as queryable
	// fields for dynamic return values.
	Enemies func(c Dog) ([]Cat, error)
}

var cats = []Cat{
	{Name: "Maru", Age: 3, Color: "White"},
	{Name: "Hana", Age: 1, Color: "Gray"},
	{Name: "Lily", Age: 2, Color: "Black"},
}

var dogs = []Dog{
	{
		Name:   "Bello",
		Age:    2,
		Color:  "Black",
		Friend: cats[0],
		Enemies: func(self Dog) ([]Cat, error) {
			// Functions can be used as queryable
			// fields for dynamic return values.

			// Bello hates Hana and Lily on Mondays.
			if time.Now().Weekday() == time.Monday {
				return cats[1:2], nil
			} else {
				return []Cat{}, nil
			}
		},
	},
	{
		Name:    "Momo",
		Age:     3,
		Color:   "White",
		Friend:  cats[0],
		Enemies: nil, // has no enemies
	},
	{
		Name:    "Kuro",
		Age:     1,
		Color:   "Gray",
		Friend:  cats[1],
		Enemies: nil, // has no enemies
	},
}

func QueryDogs(c echo.Context) error {
	var post struct {
		Query string `json:"query"`
	}
	if err := c.Bind(&post); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}

	b, err := QueryStructViaGraphql("dogs", dogs, post.Query)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}

	return c.String(http.StatusOK, string(b))
}

func QueryCats(c echo.Context) error {
	var post struct {
		Query string `json:"query"`
	}
	if err := c.Bind(&post); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}

	b, err := QueryStructViaGraphql("cats", cats, post.Query)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}

	return c.String(http.StatusOK, string(b))
}

func main() {
	e := echo.New()

	e.POST("/dogs", QueryDogs)
	e.POST("/cats", QueryCats)

	e.Logger.Fatal(e.Start(":8000"))
}
