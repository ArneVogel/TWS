package main

import (
	_ "github.com/lib/pq"
	"database/sql"
	"fmt"
	"flag"
	"github.com/joshbetz/config"
	"html/template"
	"net/http"
	"time"
	"github.com/gorilla/mux"
)

type Page struct {
	Title string
	Description string
	Body  template.HTML
	Nav template.HTML
	Javascript template.HTML
}


// can be set with -config and -debug
var configFile string
var debug bool

// Loaded from config.json
var DB_USER string = "tws"
var DB_NAME string = "tws"
var DB_PASSWORD string = "tws"

func dbReader(query string) *sql.Rows {
	dbinfo := fmt.Sprintf("user=%s password=%s dbname=%s sslmode=disable", DB_USER, DB_PASSWORD, DB_NAME)
	db, err := sql.Open("postgres", dbinfo)
	defer db.Close()
	if err != nil {
		fmt.Println("Couln't open db in dbReader: ", err)
		return nil
	}
	rows, err := db.Query(query)
	if err != nil {
		fmt.Println("Error during query: ", err)
		return nil
	}
	return rows
}

func dbGetAggregation(rows *sql.Rows) int {
	var number int
	rows.Next()
	rows.Scan(&number)
	return number
}

func createNavbar(links ...string) template.HTML {
	nav := `<nav class="blue">
	<div class="container">
		<div class="nav-wrapper">
		  <div class="col s12">`
	buildUp := ""
	for _, word := range links {
		buildUp += "/" + word
		nav += fmt.Sprintf(`<a href="%v" class="breadcrumb">%v</a>`, buildUp, word)
	}
	nav += ` </div>
		</div>
		</div>
	</nav>`
	return template.HTML(nav)
}

func createMonthTable(rows *sql.Rows, linkTo string) template.HTML {
	table := "<table>\n<thead>\n<tr>\n"
	table += "<tr><th>month</th><th>#subs</th></tr>"
	table += "</tr>\n</thead><tbody>\n"

	for rows.Next() {
		table += "<tr>\n"
		
		var count, month string
		
		rows.Scan(&count, &month)
		table += fmt.Sprintf(`<td><a href="/streamer/%v/%v">%v</a></td><td>%v</td>`, linkTo, month, month, count)
		
		table += "</tr>\n"
	}
	table += "</tbody></table>"
	return template.HTML(table)
}

func renderTemplate(w http.ResponseWriter, tmpl string, p *Page) {
    t, _ := template.ParseFiles(tmpl + ".html")
    t.Execute(w, p)
}

func createUserTable(rows *sql.Rows, linkTo string) template.HTML {
	table := "<table>\n<thead>\n<tr>\n"
	table += "<tr><th>" + linkTo + "</th><th>subscription</th><th>month</th><th>date</th><th>msg</th></tr>"
	table += "</tr>\n</thead><tbody>\n"
	
	for rows.Next() {
		table += "<tr>\n"
		
		var display_name, name, subscription, msg string
		var month int
		var date time.Time
		
		rows.Scan(&display_name, &name, &subscription, &month, &date, &msg)
		table += fmt.Sprintf(`<td><a href="/%v/%v">%v</a></td><td>%v</td><td>%v</td><td>%v</td><td>%v</td>`, linkTo, name, display_name, subscription, month, fmt.Sprintf(date.Format("2006-01-02")), msg)
		table += "</tr>\n"
	}
	table += "</tbody></table>"
	return template.HTML(table)
}

func userHandler(w http.ResponseWriter, r *http.Request) {
	var page Page
	query := `select p.display_name, p.name as streamer,
	CASE s.sub_plan
    	WHEN '1000' THEN '4.99'
        WHEN '2000' THEN '9.99'
        WHEN '3000' THEN '24.99'
        ELSE 'Prime'
    END as subscription,
 s.month, s.date, s.msg
 from partner p join subscription s on p_id = p.id join users u on u_id = u.id 
 where u.name = '%s' and date > current_date - interval '30' day`
	query2 := `select p.display_name, p.name as streamer,
	CASE s.sub_plan
    	WHEN '1000' THEN '4.99'
        WHEN '2000' THEN '9.99'
        WHEN '3000' THEN '24.99'
        ELSE 'Prime'
    END as subscription,
 s.month, s.date, s.msg
 from partner p join subscription s on p_id = p.id join users u on u_id = u.id 
 where u.name = '%s' and date < current_date - interval '30' day`
	user := r.URL.Path[len("/user/"):]
	page.Title = "Subscriptions from " + user
	page.Description = "Subscriptions from " + user + " on twitch"
	page.Body = template.HTML("Active subscriptions from " + user + ":\n")
	page.Body += createUserTable(dbReader(fmt.Sprintf(query, user)), "streamer")
	page.Body += template.HTML("\nOld subscriptions from " + user + ":\n")
	page.Body += createUserTable(dbReader(fmt.Sprintf(query2, user)), "streamer")
	page.Nav = createNavbar("user", user)
	renderTemplate(w, "view", &page)
}


func streamerMonthHandler(w http.ResponseWriter, r *http.Request) {
	var page Page
	query := `select u.display_name, u.name, CASE s.sub_plan
    	WHEN '1000' THEN '$4.99'
        WHEN '2000' THEN '$9.99'
        WHEN '3000' THEN '$24.99'
        ELSE 'Prime'
    END, s.month, s.date, s.msg from partner p join subscription s on s.p_id = p.id join users u on u.id = s.u_id 
    where p.name = '%s' and s.date::text like '%s-__'`
	querySum := `select count(partner.id) from partner join subscription s on partner.id = p_id where partner.name = '%s'and s.date::text like '%s-__'`
	params := mux.Vars(r)
	streamer := params["name"]
	month := params["month"]
	subs :=  dbGetAggregation(dbReader(fmt.Sprintf(querySum, streamer, month)))
	page.Title = "Subscriber from " + streamer + " in " + month 
	page.Description = "Subscriber from " + streamer + " on Twitch in " + month
	page.Body = template.HTML(fmt.Sprintf(`Subscriber from %v in %v, total: %v`, streamer, month, subs))
	page.Body += createUserTable(dbReader(fmt.Sprintf(query, streamer, month)), "user")
	page.Nav = createNavbar("streamer", streamer, month)
	renderTemplate(w, "view", &page)
}

func streamerProfileHandler(w http.ResponseWriter, r *http.Request) {
	var page Page
	query := `select count(s.id) as subs, TO_CHAR(s.date, 'YYYY-MM') as month from subscription s join partner p on p.id = p_id where p.name = '%s' group by TO_CHAR(s.date, 'YYYY-MM')`
	params := mux.Vars(r)
	streamer := params["name"]
	page.Title = "Subscriber from " + streamer
	page.Description = "Subscriber from " + streamer + " on Twitch"
	page.Body = createMonthTable(dbReader(fmt.Sprintf(query, streamer)), streamer)
	page.Nav = createNavbar("streamer", streamer)
	renderTemplate(w, "view", &page)
}

func createTopTable(rows *sql.Rows, linkTo string) template.HTML {
	table := "<table>\n<thead>\n<tr>\n"
	table += "<tr><th>Streamer</th><th>#subs</th></tr>"
	table += "</tr>\n</thead><tbody>\n"
	
	var person string
	var total int
	
	for rows.Next() {
		table += "<tr>\n"
		rows.Scan(&total, &person)
		table += fmt.Sprintf(`<td><a href="/%v/%v">%v</a></td><td>%v</td>`, linkTo, person, person, total)
		table += "</tr>\n"
	}
	rows.Close()
	table += "</tbody></table>"
	return template.HTML(table)
}

func topUserHandler(w http.ResponseWriter, r *http.Request) {
	var page Page
	queryAll := `select count(u.id), u.name from users u join subscription s on u_id = u.id group by u.id order by count DESC limit 25`
	queryMonth := `select count(u.id), u.name from users u join subscription s on u_id = u.id where date < (current_date - interval '30') group by u.id order by count DESC limit 25`
	page.Title = "Users with the most Subscriptions"
	page.Description = "Users with the most subscriptions on twitch"
	page.Body = template.HTML("<h2>All time</h2>")
	page.Body += createTopTable(dbReader(queryAll), "user")
	page.Body += template.HTML("<h2>This month</h2>")
	page.Body += createTopTable(dbReader(queryMonth), "user")
	page.Nav = createNavbar("user")
	renderTemplate(w, "view", &page)
}

func topStreamerHandler(w http.ResponseWriter, r *http.Request) {
	var page Page
	queryAll := `select count(p.id), p.name from partner p join subscription s on p_id = p.id group by p.id order by count DESC limit 25`
	queryMonth := `select count(p.id), p.name from partner p join subscription s on p_id = p.id where date < (current_date - interval '30') group by p.id order by count DESC limit 25`
	page.Title = "Streamer with the most subscribers on twitch"
	page.Description = "Streamer with the most subscriber on twitch"
	page.Body = template.HTML("<h2>All time</h2>")
	page.Body += createTopTable(dbReader(queryAll), "streamer")
	page.Body += template.HTML("<h2>This month</h2>")
	page.Body += createTopTable(dbReader(queryMonth), "streamer")
	page.Nav = createNavbar("streamer")
	renderTemplate(w, "view", &page)
}

func getNames(rows *sql.Rows) []string {
	var names []string
	for rows.Next() {
		var name string
		err := rows.Scan(&name)
		if err != nil {
			fmt.Println("error in getPartnersNames: ", err)
			return nil
		}
		names = append(names, name)
	}
	return names
}

func createAutocompleteJson(names []string, linkTo string) string {
	result := ""
	for _, name := range names {
		result += fmt.Sprintf(`{"text": "%s", "website-link": "/%s/%s"},`, name, linkTo, name)
	}
	return result
}

func frontHandler(w http.ResponseWriter, r *http.Request) {
	var page Page
	page.Title = "Subscription Data"
	
	page.Javascript = `<script>
jQuery( document ).ready(function($) {

var streamer = {
    data: [
        `
	page.Javascript += template.HTML(createAutocompleteJson(getNames(dbReader("SELECT name FROM partner")), "streamer"))
	page.Javascript += `
    ],

    getValue: "text",

    template: {
        type: "links",
        fields: {
            link: "website-link"
        }
    }
};

$("#streamer").easyAutocomplete(streamer);
   
});

</script>`
	
	page.Description = "Subscription Data from Twitch"
	renderTemplate(w, "front", &page)
  
	
  
}

func main() {
	rtr := mux.NewRouter()
	rtr.HandleFunc("/streamer/{name:[a-z0-9_]+}/{month:[a-z0-9-]+}", streamerMonthHandler)
	rtr.HandleFunc("/streamer/{name:[a-z0-9_]+}", streamerProfileHandler)
	rtr.HandleFunc("/user/{name:[a-z0-9_]+}", userHandler)
	rtr.HandleFunc("/streamer", topStreamerHandler)
	rtr.HandleFunc("/user", topUserHandler)
	rtr.HandleFunc("/", frontHandler)
	http.Handle("/", rtr)
    http.ListenAndServe(":3000", nil)
}

func init() {
	configFlag := flag.String("config", "../config.json", "use -config if the config file is not named config.json or is in a different folder than the tws binary")
	debugFlag := flag.Bool("debug", false, "enable debug output")
	flag.Parse()
	
	debug = *debugFlag
	configFile = *configFlag
	
	c := config.New(configFile)
	err := c.Get("DN_NAME", &DB_NAME)
	if err != nil && debug {
		fmt.Println("Couldn't get DB_NAME from config file: ", configFile)
	}
	err = c.Get("DB_USER", &DB_USER)
	if err != nil && debug {
		fmt.Println("Couldn't get DB_USER from config file: ", configFile)
	}
	err = c.Get("DB_PASSWORD", &DB_PASSWORD)
	if err != nil && debug {
		fmt.Println("Couldn't get DB_PASSWORD from config file: ", configFile)
	}
	
	if debug {
		fmt.Printf("DB_NAME: %v, DB_USER: %v, DB_PASSWORD: %v\n", DB_NAME, DB_USER, DB_PASSWORD)
	}
}
