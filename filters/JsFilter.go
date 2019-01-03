package filters

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/kataras/iris/core/errors"

	"gitex.labbs.com.br/labbsr0x/proxy/go-horse/plugins"
	"gitex.labbs.com.br/labbsr0x/proxy/go-horse/util"

	"github.com/kataras/iris"
	"github.com/rs/zerolog/log"

	"gitex.labbs.com.br/labbsr0x/proxy/go-horse/config"
	"github.com/robertkrimen/otto"
)

var client = &http.Client{}

// BodyOperation : Read or Write > If the body content was updated by the filter and needs to be overwritten in the response,  Write property should be passed
type BodyOperation int

const (
	// Read : no changes, keep the original
	Read BodyOperation = 0
	// Write : changes made, override the old
	Write BodyOperation = 1
)

// Invoke : a property to tell if the filter is gonna be executed before (Request) or after (Response) the client request be send to the docker daemon
type Invoke int

const (
	// Response filter invoke on the response from the docker daemon
	Response Invoke = 0
	// Request filter invoke on the request from the docker client
	Request Invoke = 1
)

// JsFilterModel javascript filter model
type JsFilterModel struct {
	Name        string
	Order       int
	PathPattern string
	Invoke      Invoke
	Function    string
	regex       *regexp.Regexp
}

// JsFilterFunctionReturn the return from the javascript filter execution
type JsFilterFunctionReturn struct {
	Next      bool
	Body      string
	Status    int
	Operation BodyOperation
	Err       error
}

// MatchURL tells if the filter should be executed for the URL in the context
func (jsFilter JsFilterModel) MatchURL(ctx iris.Context) bool {
	if jsFilter.regex == nil {
		regex, error := regexp.Compile(jsFilter.PathPattern)
		if error != nil {
			log.Error().Str("plugin_name", jsFilter.Name).Err(error).Msg("Error compiling the filter url matcher regex")
		} else {
			jsFilter.regex = regex
		}
	}
	return jsFilter.regex.MatchString(ctx.RequestPath(false))
}

// Exec run the filter
func (jsFilter JsFilterModel) Exec(ctx iris.Context, body string) (JsFilterFunctionReturn, error) {

	js := otto.New()

	if body == "" {
		body = "{}"
	}

	funcRet, error := js.Call("JSON.parse", nil, body)
	if error != nil {
		log.Error().Str("plugin_name", jsFilter.Name).Err(error).Msg("Error parsing body string to JS object - js filter exec")
		emptyBody, _ := js.Object("({})")
		funcRet, _ = otto.ToValue(emptyBody)
	}

	operation, error := js.Object("({READ : 0, WRITE : 1})")
	if error != nil {
		log.Error().Str("plugin_name", jsFilter.Name).Err(error).Msg("Error creating operation object - js filter exec")
	}

	ctxJsObj, _ := js.Object("({})")

	ctxJsObj.Set("url", ctx.Request().URL.String())
	ctxJsObj.Set("body", funcRet.Object())
	ctxJsObj.Set("operation", operation)
	ctxJsObj.Set("method", strings.ToUpper(ctx.Method()))
	ctxJsObj.Set("getVar", func(call otto.FunctionCall) otto.Value { return requestScopeGetToJSContext(ctx, call) })
	ctxJsObj.Set("setVar", func(call otto.FunctionCall) otto.Value { return requestScopeSetToJSContext(ctx, call) })
	ctxJsObj.Set("listVar", func(call otto.FunctionCall) otto.Value { return requestScopeListToJSContext(ctx, call) })
	ctxJsObj.Set("headers", ctx.Request().Header)
	ctxJsObj.Set("request", httpRequestTOJSContext)

	js.Set("ctx", ctxJsObj)

	pluginsJsObj, _ := js.Object("({})")

	for _, jsPlugin := range plugins.JSPluginList {
		error := pluginsJsObj.Set(jsPlugin.Name(), func(call otto.FunctionCall) otto.Value { return jsPlugin.Set(ctx, call) })
		if error != nil {
			log.Error().Str("plugin_name", jsPlugin.Name()).Err(error).Msg("Error on applying GO->JS plugin - js filter exec")
		}
	}

	js.Set("plugins", pluginsJsObj)

	returnValue, error := js.Run("(" + jsFilter.Function + ")(ctx, plugins)")

	if error != nil {
		log.Error().Str("plugin_name", jsFilter.Name).Err(error).Msg("Error executing filter - js filter exec")
		return JsFilterFunctionReturn{Next: false, Body: "{\"message\" : \"Error from docker daemon proxy go-horse : \"" + error.Error() + "}"}, error
	}

	result := returnValue.Object()

	jsFunctionReturn := JsFilterFunctionReturn{}

	if value, err := result.Get("next"); err == nil {
		if value, err := value.ToBoolean(); err == nil {
			jsFunctionReturn.Next = value
		} else {
			return errorReturnFilter(error)
		}
	} else {
		return errorReturnFilter(error)
	}

	if value, err := result.Get("body"); err == nil {
		if value, err := js.Call("JSON.stringify", nil, value); err == nil {
			jsFunctionReturn.Body = value.String()
		} else {
			return errorReturnFilter(error)
		}
	} else {
		return errorReturnFilter(error)
	}

	if value, err := result.Get("operation"); err == nil {
		if value, err := value.ToInteger(); err == nil {
			if value == 1 {
				jsFunctionReturn.Operation = Write
			} else {
				jsFunctionReturn.Operation = Read
			}
		} else {
			return errorReturnFilter(error)
		}
	} else {
		return errorReturnFilter(error)
	}

	if value, err := result.Get("status"); err == nil {
		if value, err := value.ToInteger(); err == nil {
			jsFunctionReturn.Status = int(value)
		} else {
			return errorReturnFilter(error)
		}
	} else {
		return errorReturnFilter(error)
	}

	if value, err := result.Get("error"); err == nil {
		if value.IsDefined() {
			if value, err := value.ToString(); err == nil {
				jsFunctionReturn.Err = errors.New(value)
			} else {
				return errorReturnFilter(error)
			}
		}
	} else {
		return errorReturnFilter(error)
	}
	// wierd
	return jsFunctionReturn, jsFunctionReturn.Err
}

func errorReturnFilter(error error) (JsFilterFunctionReturn, error) {
	log.Error().Err(error).Msg("Error parsing filter return value - js filter exec")
	return JsFilterFunctionReturn{Body: "{\"message\" : \"Proxy error : \"" + error.Error() + "}"}, error
}

func requestScopeGetToJSContext(ctx iris.Context, call otto.FunctionCall) otto.Value {
	key, error := call.Argument(0).ToString()
	if error != nil {
		log.Error().Err(error).Msg("Error parsing requestScopeGetToJSContext key field - js filter exec")
	}
	value := util.RequestScopeGet(ctx, key)
	result, error := otto.ToValue(value)
	if error != nil {
		log.Error().Err(error).Msg("Error parsing requestScopeGetToJSContext function return - js filter exec")
	}
	return result
}

func requestScopeSetToJSContext(ctx iris.Context, call otto.FunctionCall) otto.Value {
	key, error := call.Argument(0).ToString()
	if error != nil {
		log.Error().Err(error).Msg("Error parsing requestScopeSetToJSContext key field - js filter exec")
	}
	value, error := call.Argument(1).ToString()
	if error != nil {
		log.Error().Err(error).Msg("Error parsing requestScopeSetToJSContext function exec - js filter exec")
	}
	util.RequestScopeSet(ctx, key, value)
	return otto.NullValue()
}

func requestScopeListToJSContext(ctx iris.Context, call otto.FunctionCall) otto.Value {
	mapa := util.RequestScopeList(ctx)
	result, error := call.Otto.ToValue(mapa)
	if error != nil {
		log.Error().Err(error).Msg("Error parsing requestScopeListToJSContext response map - js filter exec")
	}
	return result
}

func httpRequestTOJSContext(call otto.FunctionCall) otto.Value {
	method, error := call.Argument(0).ToString()
	if error != nil {
		log.Error().Err(error).Msg("Error parsing httpRequestTOJSContext method - js filter exec")
	}
	url, error := call.Argument(1).ToString()
	if error != nil {
		log.Error().Err(error).Msg("Error parsing httpRequestTOJSContext url - js filter exec")
	}
	body, error := call.Argument(2).ToString()
	if error != nil {
		log.Error().Err(error).Msg("Error parsing httpRequestTOJSContext body - js filter exec")
	}
	var req *http.Request
	var err interface{}

	if method == "GET" {
		req, err = http.NewRequest(method, url, nil)
		if err != nil {
			log.Error().Err(error).Msg("Error parsing httpRequestTOJSContext GET header - js filter exec")
		}
	} else {
		req, err = http.NewRequest(method, url, strings.NewReader(body))
		if err != nil {
			log.Error().Err(error).Msg("Error parsing httpRequestTOJSContext OTHER THAN GET header - js filter exec")
		}
	}

	headers := call.Argument(3).Object()
	if headers != nil {
		for _, key := range headers.Keys() {
			header, error := headers.Get(key)
			if error != nil {
				log.Error().Err(error).Msg("Error parsing httpRequestTOJSContext GET header - js filter exec")
			}
			headerValue, error := header.ToString()
			if error != nil {
				log.Error().Err(error).Msg("Error parsing httpRequestTOJSContext GET header - js filter exec")
			}
			req.Header.Add(key, headerValue)
		}
	}
	log.Debug().Str("method", method).Str("url", url).Str("body", body).Str("headers", fmt.Sprintf("%#v", headers)).Msg("Request parameters")
	resp, err := client.Do(req)
	if err != nil {
		log.Error().Msg("Error executing the request - httpRequestTOJSContext " + fmt.Sprintf("%#v", err))
		response, _ := call.Otto.Object("({})")
		buf, marshalError := json.Marshal(err)
		if marshalError == nil {
			response.Set("body", fmt.Sprintf("%v", string(buf)))
		} else {
			response.Set("body", fmt.Sprintf("%#v", err))
		}
		response.Set("status", 0)
		value, _ := otto.ToValue(response)
		return value
	}
	defer resp.Body.Close()
	if req.Body != nil {
		defer req.Body.Close()
	}

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Error().Msg("Error parsing request body - httpRequestTOJSContext " + fmt.Sprintf("%#v", err))
	}

	response, _ := call.Otto.Object("({})")

	bodyObjectJs, error := call.Otto.ToValue(string(bodyBytes))
	if error != nil {
		log.Error().Err(error).Msg("Error parsing response body to JS object - httpRequestTOJSContext")
	}

	headersObjectJs, error := call.Otto.ToValue(resp.Header)
	if error != nil {
		log.Error().Err(error).Msg("Error parsing response headers to JS object - httpRequestTOJSContext")
	}

	response.Set("body", bodyObjectJs)
	response.Set("status", resp.StatusCode)
	response.Set("headers", headersObjectJs)

	value, _ := otto.ToValue(response)

	return value
}

// Load load the filter from files
func Load() []JsFilterModel {
	return parseFilterObject(readFromFile())
}

func readFromFile() map[string]string {

	var jsFilterFunctions = make(map[string]string)

	files, err := ioutil.ReadDir(config.JsFiltersPath)
	if err != nil {
		log.Error().Err(err).Msg("Error reading filters dir - readFromFile")
	}

	for _, file := range files {
		content, error := ioutil.ReadFile(config.JsFiltersPath + "/" + file.Name())
		if error != nil {
			log.Error().Err(err).Str("file", file.Name()).Msg("Error reading filter filter - readFromFile")
			continue
		}
		jsFilterFunctions[file.Name()] = string(content)
		log.Debug().Str("file", file.Name()).Str("filter_content", string(content)).Msg("js filter - readFromFile")
	}

	return jsFilterFunctions
}

func parseFilterObject(jsFilterFunctions map[string]string) []JsFilterModel {
	var filterMoldels []JsFilterModel

	fileNamePattern := regexp.MustCompile("^([0-9]{1,3})\\.(request|response)\\.(.*?)\\.js$")

	for fileName, jsFunc := range jsFilterFunctions {
		nameProperties := fileNamePattern.FindStringSubmatch(fileName)
		if nameProperties == nil || len(nameProperties) < 4 {
			log.Error().Str("file", fileName).Msg("Error file name")
			continue
		}

		order := nameProperties[1]
		invokeTime := nameProperties[2]
		name := nameProperties[3]

		js := otto.New()

		funcFilterDefinition, error := js.Call("(function(){return"+jsFunc+"})", nil, nil)
		if error != nil {
			log.Error().Err(error).Str("file", fileName).Msg("Error on JS object definition - parseFilterObject")
			continue
		}

		filter := funcFilterDefinition.Object()

		filterDefinition := JsFilterModel{}

		if invokeTime == "request" {
			filterDefinition.Invoke = Request
		} else {
			filterDefinition.Invoke = Response
		}

		oderInt, orderParserError := strconv.Atoi(order)
		if orderParserError != nil {
			log.Error().Err(error).Str("file", fileName).Msg("Error on order int conversion - parseFilterObject")
			continue
		}
		filterDefinition.Order = oderInt
		filterDefinition.Name = name

		if value, err := filter.Get("pathPattern"); err == nil {
			if value, err := value.ToString(); err == nil {
				filterDefinition.PathPattern = value
			} else {
				log.Error().Err(err).Str("file", fileName).Str("field", "pathPattern").Msg("Error on JS filter definition - parseFilterObject")
			}
		} else {
			log.Error().Err(err).Str("file", fileName).Str("field", "pathPattern").Msg("Error on JS filter definition - parseFilterObject")
		}

		if value, err := filter.Get("function"); err == nil {
			if value, err := value.ToString(); err == nil {
				filterDefinition.Function = value
			} else {
				log.Error().Err(err).Str("file", fileName).Str("field", "function").Msg("Error on JS filter definition - parseFilterObject")
			}
		} else {
			log.Error().Err(err).Str("file", fileName).Str("field", "function").Msg("Error on JS filter definition - parseFilterObject")
		}

		filterMoldels = append(filterMoldels, filterDefinition)
	}
	return filterMoldels
}
