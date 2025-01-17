// Package plugin implements functions and types for maintaining and running
// stash plugins.
//
// Stash plugins are configured using yml files in the configured plugins
// directory. These yml files must follow the Config structure format.
//
// The main entry into the plugin sub-system is via the Cache type.
package plugin

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/manager/config"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/plugin/common"
)

// Cache stores plugin details.
type Cache struct {
	config     *config.Instance
	plugins    []Config
	gqlHandler http.HandlerFunc
}

// NewCache returns a new Cache.
//
// Plugins configurations are loaded from yml files in the plugin
// directory in the config and any subdirectories.
//
// Does not load plugins. Plugins will need to be
// loaded explicitly using ReloadPlugins.
func NewCache(config *config.Instance) *Cache {
	return &Cache{
		config: config,
	}
}

func (c *Cache) RegisterGQLHandler(handler http.HandlerFunc) {
	c.gqlHandler = handler
}

// LoadPlugins clears the plugin cache and loads from the plugin path.
// In the event of an error during loading, the cache will be left empty.
func (c *Cache) LoadPlugins() error {
	c.plugins = nil
	plugins, err := loadPlugins(c.config.GetPluginsPath())
	if err != nil {
		return err
	}

	c.plugins = plugins
	return nil
}

func loadPlugins(path string) ([]Config, error) {
	plugins := make([]Config, 0)

	logger.Debugf("Reading plugin configs from %s", path)
	pluginFiles := []string{}
	err := filepath.Walk(path, func(fp string, f os.FileInfo, err error) error {
		if filepath.Ext(fp) == ".yml" {
			pluginFiles = append(pluginFiles, fp)
		}
		return nil
	})

	if err != nil {

		return nil, err
	}

	for _, file := range pluginFiles {
		plugin, err := loadPluginFromYAMLFile(file)
		if err != nil {
			logger.Errorf("Error loading plugin %s: %s", file, err.Error())
		} else {
			plugins = append(plugins, *plugin)
		}
	}

	return plugins, nil
}

// ListPlugins returns plugin details for all of the loaded plugins.
func (c Cache) ListPlugins() []*models.Plugin {
	var ret []*models.Plugin
	for _, s := range c.plugins {
		ret = append(ret, s.toPlugin())
	}

	return ret
}

// ListPluginTasks returns all runnable plugin tasks in all loaded plugins.
func (c Cache) ListPluginTasks() []*models.PluginTask {
	var ret []*models.PluginTask
	for _, s := range c.plugins {
		ret = append(ret, s.getPluginTasks(true)...)
	}

	return ret
}

// CreateTask runs the plugin operation for the pluginID and operation
// name provided. Returns an error if the plugin or the operation could not be
// resolved.
func (c Cache) CreateTask(pluginID string, operationName string, serverConnection common.StashServerConnection, args []*models.PluginArgInput, progress chan float64) (Task, error) {
	// find the plugin and operation
	plugin := c.getPlugin(pluginID)

	if plugin == nil {
		return nil, fmt.Errorf("no plugin with ID %s", pluginID)
	}

	operation := plugin.getTask(operationName)
	if operation == nil {
		return nil, fmt.Errorf("no task with name %s in plugin %s", operationName, plugin.getName())
	}

	task := pluginTask{
		plugin:           plugin,
		operation:        operation,
		serverConnection: serverConnection,
		args:             args,
		progress:         progress,
		gqlHandler:       c.gqlHandler,
	}
	return task.createTask(), nil
}

func (c Cache) getPlugin(pluginID string) *Config {
	for _, s := range c.plugins {
		if s.id == pluginID {
			return &s
		}
	}

	return nil
}
