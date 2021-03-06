package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/bluele/gforms"
)

var (
	listenAddr       = flag.String("l", ":8081", "Address to listen on")
	answersDirectory = flag.String("d", "questionnaire-answers", "Directory to store questionaire answers")
	webRoot          = flag.String("r", "/", "Web root for links")
)

func check(e error) {
	if e != nil {
		panic(e)
	}
}

//StringToHTML takes a string and returns HTML
func StringToHTML(s string) template.HTML {
	return template.HTML(s)
}

//given a question q, will define its validators from given fields
func defineValidators(q map[string]interface{}, locale string) gforms.Validators {
	dat, err := ioutil.ReadFile("/opt/questionnaire/messages.json")
	check(err)
	var errors map[string]interface{}
	err = json.Unmarshal(dat, &errors)
	check(err)

	var validators []gforms.Validator

	errorMessages := errors[locale].(map[string]interface{})
	if q["required"].(bool) {
		validators = append(validators, gforms.Required(errorMessages["required"].(string)))
	}
	if v, ok := q["maxLength"].(float64); ok {
		validators = append(validators, gforms.MaxLengthValidator(int(v), fmt.Sprintf(errorMessages["maxChars"].(string), int(v))))
	}

	return gforms.Validators(validators)
}

//given an input string 'locale' returns a valid locale (english by default)
func getLocale(locale string) string {
	validLocales := [...]string{"en", "ar"}

	for _, l := range validLocales {
		if l == locale {
			return l
		}
	}
	return "en"
}

//given a map of questions, return a fields list (gforms.Field)
func getFields(questions []interface{}, locale string) []gforms.Field {
	var fields []gforms.Field

	for _, question := range questions {
		q := question.(map[string]interface{})
		switch q["type"].(string) {
		case "textBoxQuestion":
			fields = append(fields, gforms.NewTextField(
				q["question"].(string),
				defineValidators(q, locale),
			))
		case "numberQuestion":
			fields = append(fields, gforms.NewIntegerField(
				q["question"].(string),
				defineValidators(q, locale),
			))
		case "multipleChoiceQuestion":
			fields = append(fields, gforms.NewMultipleTextField(q["question"].(string),
				defineValidators(q, locale),
				gforms.CheckboxMultipleWidget(
					map[string]string{
						"class": q["name"].(string),
						"id":    q["name"].(string),
					},
					func() gforms.CheckboxOptions {
						var retval [][]string
						for _, v := range q["choices"].([]interface{}) {
							retval = append(retval, []string{
								v.(string), v.(string), "false", "false",
							})
						}
						return gforms.StringCheckboxOptions(retval)
					}),
			))
		case "singleChoiceQuestion":
			fields = append(fields, gforms.NewTextField(q["question"].(string),
				defineValidators(q, locale),
				gforms.RadioSelectWidget(
					map[string]string{
						"class": q["name"].(string),
						"id":    q["name"].(string),
					},
					func() gforms.RadioOptions {
						var retval [][]string
						for _, v := range q["choices"].([]interface{}) {
							retval = append(retval, []string{
								v.(string), v.(string), "false", "false",
							})
						}
						return gforms.StringRadioOptions(retval)
					}),
			))
		}
	}
	return fields
}

func getMessage(locale string, messageString string) string {
	dat, err := ioutil.ReadFile("/opt/questionnaire/messages.json")
	check(err)
	var messages map[string]interface{}
	err = json.Unmarshal(dat, &messages)
	check(err)

	message := messages[locale].(map[string]interface{})[messageString].(string)
	return message
}

func getThankYouMessage(locale string) thankYouMessage {
	message := getMessage(locale, "thankYou")
	message = fmt.Sprintf(message, "/")
	return thankYouMessage{StringToHTML(message)}
}

type thankYouMessage struct {
	Message template.HTML
}

type templateContext struct {
	Message          string
	RequiredFields   string
	ScaleExplanation string
	Form             *gforms.FormInstance
	Root             string
}

func questionnaireHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	locale := strings.SplitAfterN(r.URL.Path, "/", 2)[1]
	locale = getLocale(locale)

	//load the json file with questions
	dat, err := ioutil.ReadFile("/opt/questionnaire/questions.json")
	check(err)
	var questions map[string]interface{}
	err = json.Unmarshal(dat, &questions)
	check(err)

	//create a fields list
	fields := getFields(questions[locale].([]interface{}), locale)

	// prepare the form
	userForm := gforms.DefineForm(gforms.NewFields(fields...))
	form := userForm(r)

	//parse the request
	if r.Method == "GET" || !form.IsValid() {
		//prepare the template for the form
		funcMap := template.FuncMap{
			"stringToHTML": StringToHTML,
		}
		t := template.Must(template.New("questionnaire.html").Funcs(funcMap).ParseFiles("/opt/questionnaire/questionnaire.html"))
		t.Execute(w, templateContext{
			getMessage(locale, "questionnaireExplanation"),
			getMessage(locale, "requiredFields"),
			getMessage(locale, "scaleExplanation"),
			form,
			*webRoot})
		return
	}

	//if the execution comes to here, it means we can record answers
	//dump the question answers into a json
	fmt.Println(form.CleanedData)
	jsonString, err := json.Marshal(form.CleanedData)
	check(err)
	timeString := fmt.Sprintf("%d-%02d-%02dT%02d:%02d:%02d",
		time.Now().Year(), time.Now().Month(), time.Now().Day(),
		time.Now().Hour(), time.Now().Minute(), time.Now().Second())
	f, err := os.Create(*answersDirectory + "/" + timeString + ".json")
	defer f.Close()
	check(err)
	f.WriteString(string(jsonString))

	//return a thank you view
	t := template.Must(template.New("thankYou.html").ParseFiles("/op/questionnaire/thankYou.html"))
	t.Execute(w, getThankYouMessage(locale))

}

func main() {
	flag.Parse()
	http.HandleFunc("/", questionnaireHandler)
	log.Println("Listening on", *listenAddr)
	log.Fatal(http.ListenAndServe(*listenAddr, nil))
}
