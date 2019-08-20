package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/gocolly/colly"
	_ "github.com/mattn/go-sqlite3"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"
)

type item struct {
	ImageURL    string    `json:"image_url"`
	Description string    `json:"description"`
	Price       []float32 `json:"price"`
	ProductURL  string    `json:"product_url"`
	Title       string    `json:"title"`
}

type categoryResponse struct {
	Items []item `json:"items"`
	Meta  struct {
		Total int `json:"total"`
	} `json:"meta"`
}

func insertProduct(product item, db *sql.DB) {
	res, err := db.Query("SELECT Count(*) FROM products WHERE title=?", product.Title)
	if err != nil {
		panic(err.Error())
	}
	var row int
	res.Scan(&row)
	if row > 0 {
		fmt.Printf("rows: %d\n", row)
	}
	res.Close()
	if row > 0 {
		fmt.Println("product already scraped: " + product.Title)
		return
	} else {
		fmt.Printf("existing products with that title: %d for title: %s\n", row, product.Title)
	}
	// stmt, err := db.Prepare("INSERT INTO products(image_url, description, price, product_url, title) values(?, ?, ?, ?, ?)")
	// if err != nil {
	// 	panic(err.Error())
	// }
	_, err = db.Exec("INSERT INTO products(image_url, description, price, product_url, title) values(?, ?, ?, ?, ?)", product.ImageURL, product.Description, product.Price[0], product.ProductURL, product.Title)
	if err != nil {
		panic(err.Error())
	}

	// stmt.Close()
}

func main() {
	signalChan := make(chan os.Signal, 1)
	// cleanupDone := make(chan struct{})
	signal.Notify(signalChan, os.Interrupt)
	running := true

	db, err := sql.Open("sqlite3", "products.db")
	go func() {
		<-signalChan
		fmt.Println("\nReceived interrupt. Quitting...")
		running = false
		time.Sleep(20 * time.Millisecond)
		db.Close()
		return
	}()
	defer db.Close()
	if err != nil {
		panic(err.Error())
	}

	db.Exec("CREATE TABLE `products` ( `image_url` TEXT, `description` TEXT, `price` NUMERIC, `product_url` TEXT, `title` TEXT )")

	c := colly.NewCollector()
	var categories []string

	c.Limit(&colly.LimitRule{
		// Filter domains affected by this rule
		DomainGlob: "thebrick.com/*",
		// Set a delay between requests to these domains
		Delay: 1 * time.Second,
		// Add an additional random delay
		RandomDelay: 1 * time.Second,
	})

	c.OnResponse(func(response *colly.Response) {
		// fmt.Println("got some response")
		// fmt.Println("status: " + string(response.StatusCode))
		// fmt.Println(string(response.Body))
	})

	c.OnHTML(".subcategory-wrapper .subcategory-column > li > a", func(e *colly.HTMLElement) {
		// fmt.Println(e.Attr("href"))
		catSplit := strings.Split(e.Attr("href"), "/")
		catName := catSplit[len(catSplit)-1]
		categories = append(categories, catName)
	})

	c.OnRequest(func(r *colly.Request) {
		// var split = strings.Split(r.URL.Path, "/")
		// fmt.Println("Visiting", split[len(split)-1])
	})

	c.OnScraped(func(r *colly.Response) { // will only have scraped the main page
		for _, category := range categories {
			if !running {
				return
			}
			fmt.Println("getting products for: " + category)
			url := "https://api-v3.findify.io/v3/smart-collection/collections/"
			url += category
			url += "?user%5Buid%5D=y752SQnRL2VxdkC5&user%5Bsid%5D=JdesrVEK3zkVqqCV&user%5Bpersist%5D=true&user%5Bexist%5D=true&t_client=1562678048638&key=9f181724-272a-41b5-9f97-800f8aa1e03f&limit=300&slot=collections%2F"
			url += category

			offset := 0

			resp, _ := http.Get(url)
			defer resp.Body.Close()
			if resp.StatusCode < 400 {
				body, _ := ioutil.ReadAll(resp.Body)
				catResponse := categoryResponse{}
				err := json.Unmarshal(body, &catResponse)
				if err != nil {
					fmt.Printf("error unmarshaling response: " + err.Error())
					panic(err.Error())
				}
				// TODO: handle products here
				for _, product := range catResponse.Items {
					insertProduct(product, db)
				}
				fmt.Printf("product total in cat: %v\n", catResponse.Meta.Total)
				for offset+300 < catResponse.Meta.Total {
					if !running {
						return
					}
					offset += 300
					fmt.Println("getting products with offset: " + strconv.Itoa(offset))
					offsetURL := url + "%5Doffset=" + string(offset)
					resp, _ := http.Get(offsetURL)
					defer resp.Body.Close()
					body, _ := ioutil.ReadAll(resp.Body)
					err := json.Unmarshal(body, &catResponse)
					if err != nil {
						fmt.Printf("error unmarshaling response: " + err.Error())
						panic(err.Error())
					}
					for _, product := range catResponse.Items {
						insertProduct(product, db)
					}
					// TODO: handle products
				}
				// fmt.Println("total items in collection %s: %d", category, len(catResponse.Items))
				// fmt.Printf("items in collection %s: %v\n", category, len(catResponse.Items))
			} else {
				fmt.Printf("Failed to load page: %d\n", resp.StatusCode)
			}
		}

	})

	c.Visit("https://www.thebrick.com/")
}
