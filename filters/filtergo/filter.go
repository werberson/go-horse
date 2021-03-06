package filtergo

import (
	"github.com/labbsr0x/go-horse/filters/model"
	"github.com/labbsr0x/go-horse/plugins"
	"github.com/kataras/iris"
	"github.com/sirupsen/logrus"
	"regexp"
)

// FilterGO Go proxy filter
type FilterGO struct {
	model.FilterConfig
	plugin plugins.GoFilterDefinition
}

// MatchURL go
func (filterGo FilterGO) MatchURL(ctx iris.Context) bool {
	return filterGo.Regex.MatchString(ctx.RequestPath(false))
}

// Config go
func (filterGo FilterGO) Config() model.FilterConfig {
	return filterGo.FilterConfig
}

// Exec go
func (filterGo FilterGO) Exec(ctx iris.Context, requestBody string) (model.FilterReturn, error) {
	return filterGo.plugin.Exec(ctx, requestBody)
}

// NewFilterGO filter factory
func NewFilterGO(plugin plugins.GoFilterDefinition) FilterGO {
	filterGo := FilterGO{}
	filterGo.plugin = plugin
	config := plugin.Config()
	regex, err := regexp.Compile(config.PathPattern)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"plugin_name": config.Name,
		}).Errorf("Error compiling the filter url matcher regex")
	}
	config.Regex = regex
	filterGo.FilterConfig = config
	return filterGo
}
