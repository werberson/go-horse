package model

import (
	"fmt"
	"reflect"
	"regexp"

	"gitex.labbs.com.br/labbsr0x/sandman-acl-proxy/plugins"

	"gitex.labbs.com.br/labbsr0x/sandman-acl-proxy/filters"
	"github.com/kataras/iris"
)

// BodyOperation json body operation type
type BodyOperation int

const (
	// Read read
	Read BodyOperation = 0
	// Write write
	Write BodyOperation = 1
)

// Invoke invoke
type Invoke int

const (
	// After After
	After Invoke = 0
	// Before After
	Before Invoke = 1
)

// Filter lero-lero
type Filter interface {
	Config() FilterConfig
	Exec(ctx iris.Context, requestBody string) FilterReturn
	MatchURL(ctx iris.Context) bool
}

// FilterConfig lero-lero
type FilterConfig struct {
	Name        string
	Order       int
	PathPattern string
	Invoke      Invoke
	Function    string
	regex       *regexp.Regexp
}

// FilterReturn lero-lero
type FilterReturn struct {
	Next      bool
	Body      string
	Status    int
	Operation BodyOperation
}

type baseFilter struct {
	Filter
	FilterConfig
}

func parseOperation(operation interface{}) BodyOperation {
	fmt.Println(">>>>>>>>>>>>>> OPERATION > ", reflect.TypeOf(operation))
	intOperation, ok := operation.(int)
	if ok {
		if intOperation == 1 {
			return Write
		}
		return Read
	}
	if operation == filters.Write {
		return Write
	}
	return Read
}

func parseInvoke(invoke interface{}) Invoke {
	fmt.Println(">>>>>>>>>>>>>> INVOKE > ", reflect.TypeOf(invoke))
	intInvoke, ok := invoke.(int)
	if ok {
		if intInvoke == 1 {
			return Before
		}
		return After
	}
	if invoke == filters.Before {
		return Before
	}
	return After
}

// FilterGO lero-lero
type FilterGO struct {
	baseFilter
	innerType plugins.Filter
}

// FilterJS lero-lero
type FilterJS struct {
	baseFilter
	innerType filters.JsFilterModel
}

// NewFilterJS lero-lero
func NewFilterJS(innerType filters.JsFilterModel) FilterJS {
	filterJs := FilterJS{}
	filterJs.innerType = innerType
	filterJs.FilterConfig = FilterConfig{Name: innerType.Name, Order: innerType.Order, PathPattern: innerType.PathPattern, Invoke: parseInvoke(innerType.Invoke), Function: innerType.Function}
	return filterJs
}

// MatchURL lero-lero
func (filterJs FilterJS) MatchURL(ctx iris.Context) bool {
	return MatchURL(ctx, filterJs.baseFilter)
}

// Config lero-lero
func (filterJs FilterJS) Config() FilterConfig {
	return filterJs.FilterConfig
}

// Exec lero-lero
func (filterJs FilterJS) Exec(ctx iris.Context, requestBody string) FilterReturn {
	jsReturn := filterJs.innerType.Exec(ctx, requestBody)
	return FilterReturn{jsReturn.Next, jsReturn.Body, jsReturn.Status, parseOperation(jsReturn.Operation)}
}

// MatchURL lero-lero
func (filterGo FilterGO) MatchURL(ctx iris.Context) bool {
	return MatchURL(ctx, filterGo.baseFilter)
}

// Config lero-lero
func (filterGo FilterGO) Config() FilterConfig {
	return filterGo.FilterConfig
}

// Exec lero-lero
func (filterGo FilterGO) Exec(ctx iris.Context, requestBody string) FilterReturn {
	Next, Body, Status, Operation := filterGo.innerType.Exec(ctx, requestBody)
	return FilterReturn{Next, Body, Status, parseOperation(Operation)}
}

// NewFilterGO lero-lero
func NewFilterGO(innerType plugins.Filter) FilterGO {
	filterGo := FilterGO{}
	filterGo.innerType = innerType
	Name, Order, PathPattern, Invoke := innerType.Config()
	filterGo.FilterConfig = FilterConfig{Name: Name, Order: Order, PathPattern: PathPattern, Invoke: parseInvoke(Invoke), Function: ""}
	return filterGo
}

// MatchURL lero lero
func MatchURL(ctx iris.Context, base baseFilter) bool {
	if base.regex == nil {
		regex, error := regexp.Compile(base.PathPattern)
		if error != nil {
			fmt.Printf("ERRO AO CRIAR REGEX PARA DAR MATCH NA URL DO FILTRO : %s; PATTERN : %s\n", base.Name, base.PathPattern)
		} else {
			base.regex = regex
		}
	}
	return base.regex.MatchString(ctx.RequestPath(false))
}