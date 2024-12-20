package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"log"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/disintegration/imaging"
	"github.com/fogleman/gg"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// Define package-level variables to act as constants
var RedColor = color.RGBA{R: 255, G: 0, B: 0, A: 255}
var GreenColor = color.RGBA{R: 0, G: 255, B: 0, A: 255}
var BlueColor = color.RGBA{R: 0, G: 0, B: 255, A: 255}
var BlackColor = color.RGBA{R: 0, G: 0, B: 0, A: 255}
var WhiteColor = color.RGBA{R: 255, G: 255, B: 255, A: 255}

type Rectangle struct {
	Left   *int `json:"left,omitempty"`
	Top    *int `json:"top,omitempty"`
	Width  *int `json:"width,omitempty"`
	Height *int `json:"height,omitempty"`
}

type Dimensions struct {
	Rectangle
}

type Offsets struct {
	XOffsetStart  *int `json:"xOffsetStart,omitempty"`
	XOffsetFinish *int `json:"xOffsetFinish,omitempty"`
	YOffsetStart  *int `json:"yOffsetStart,omitempty"`
	YOffsetFinish *int `json:"yOffsetFinish,omitempty"`
}

type ImageProperties struct {
	Center            *bool    `json:"center,omitempty"`
	Opacity           *float32 `json:"opacity,omitempty"`
	Enabled           *bool    `json:"enabled,omitempty"`
	UseAsSwitch       *bool    `json:"useAsSwitch,omitempty"`
	NeedsThrottleType *bool    `json:"needsThrottleType,omitempty"`
	Image             *image.RGBA
}

type Display struct {
	Name string `json:"name"`
	Dimensions
	Offsets
	ImageProperties
}

type Configuration struct {
	Name     string `json:"name"`
	FileName string `json:"fileName"`
	Module   *Module
	Parent   *Configuration
	Display  *Display
	Dimensions
	Offsets
	ImageProperties
	Configurations []Configuration `json:"subConfigDef"`
}

type Module struct {
	Name           string          `json:"name"`
	Tag            string          `json:"tag"`
	DisplayName    string          `json:"displayName"`
	FileName       string          `json:"fileName"`
	Category       string          `json:"category"`
	Configurations []Configuration `json:"configurations"`
}

type JSONData struct {
	Modules []Module `json:"modules"`
}

type Logger struct {
	fileName string
	file     *os.File
	mu       sync.Mutex
}

type MfdConfig struct {
	DisplayConfigurationFile string `json:"displayConfigurationFile"`
	DefaultConfiguration     string `json:"defaultConfiguration"`
	DcsSavedGamesPath        string `json:"dcsSavedGamesPath"`
	SaveCroppedImages        bool   `json:"saveCroppedImages"`
	Modules                  string `json:"modules"`
	FilePath                 string `json:"filePath"`
	UseCougar                bool   `json:"useCougar"`
	ShowRulers               bool   `json:"showRulers"`
	RulerSize                int    `json:"rulerSize"`
}

// Define the interface
type ConfigurationProvider interface {
	GetDimension() *Dimensions
	GetOffset() *Offsets
	GetImageProperties() *ImageProperties
}

type DisplayConfigurator interface {
	ConfigureDisplay() error
}

type ConfigurationProcessor interface {
	ConfigureConfiguration() error
	GetOffsetString() string
	CanCrop() bool
	GetCropRect() image.Rectangle
	GetDrawingCoordinate(newImage image.Image) image.Point
	GetDrawingArea() image.Rectangle
	GetSize() image.Point
	CenterImageWithCropAndResize(subConfigIndex int) error
}

func (config Configuration) String() string {
	return fmt.Sprintf("%s Image: %s at (%d, %d)", config.Name, config.FileName, config.Left, config.Top)
}

func ApplyConfiguration(dc DisplayConfigurator) error {
	return dc.ConfigureDisplay()
}

func (c *Configuration) GetOffsetString() string {
	return fmt.Sprintf("%d, %d, %d, %d", *c.XOffsetStart, *c.XOffsetFinish, *c.YOffsetStart, *c.YOffsetFinish)
}

func (c *Configuration) CanCrop() bool {
	rect := c.GetCropRect()
	return rect.Dx() > 0 && rect.Dy() > 0
}

func (config *Configuration) GetCropRect() image.Rectangle {
	return createRectangle(*config.XOffsetStart, *config.YOffsetStart, *config.XOffsetFinish, *config.YOffsetFinish)
}

func (config *Configuration) GetDrawingArea() image.Rectangle {
	return image.Rect(*config.Left, *config.Top, *config.Width, *config.Height)
}

func (config *Configuration) GetSize() image.Point {
	return image.Point{*config.Width, *config.Height}
}

func (config *Configuration) GetDrawingCoordinate(newImage image.Image) image.Point {
	drawPosition := image.Point{0, 0}

	// Check if the configuration has a parent
	if config.Parent != nil && config.Parent.Image != nil {
		parentBounds := config.Parent.Image.Bounds()

		// Centering logic
		if config.Center != nil && *config.Center {
			drawPosition.X = (parentBounds.Dx() - newImage.Bounds().Dx()) / 2
			drawPosition.Y = (parentBounds.Dy() - newImage.Bounds().Dy()) / 2
		} else {
			// Relative positioning logic
			if config.Left != nil {
				drawPosition.X = *config.Left
			} else {
				drawPosition.X = 0 // Default to 0 if Left is not specified
			}

			if config.Top != nil {
				drawPosition.Y = *config.Top
			} else {
				drawPosition.Y = 0 // Default to 0 if Top is not specified
			}

			// Adjust relative to parent's top-left corner
			drawPosition.X += parentBounds.Min.X
			drawPosition.Y += parentBounds.Min.Y
		}
	} else if config.Display != nil {
		// If no parent, use display dimensions for centering
		if config.Center != nil && *config.Center {
			drawPosition.X = (*config.Display.Width - newImage.Bounds().Dx()) / 2
			drawPosition.Y = (*config.Display.Height - newImage.Bounds().Dy()) / 2
		} else {
			// Default to top-left corner of the display
			drawPosition.X = 0
			drawPosition.Y = 0
		}
	}

	return drawPosition
}

func (d *Display) ConfigureDisplay() error {
	// Implementation for configuring the display
	fmt.Printf("Configuring Display: %s\n", d.Name)
	setInitialValues(d)
	return nil
}

func (c *Configuration) ConfigureConfiguration() error {
	// Implementation for configuring the display
	fmt.Printf("Configuring Configuration: %s\n", c.Name)
	setInitialValues(c)
	return nil
}

func (d *Display) GetDimension() *Dimensions {
	return &d.Dimensions
}

func (d *Display) GetOffset() *Offsets {
	return &d.Offsets
}

func (d *Display) GetImageProperties() *ImageProperties {
	return &d.ImageProperties
}

func (d *Display) GetDimensions() *Dimensions {
	return &d.Dimensions
}

func (d *Display) GetOffsets() *Offsets {
	return &d.Offsets
}

func (c *Configuration) GetDimension() *Dimensions {
	return &c.Dimensions
}

func (c *Configuration) GetOffset() *Offsets {
	return &c.Offsets
}

func (c *Configuration) GetImageProperties() *ImageProperties {
	return &c.ImageProperties
}

// LoadConfig loads the configuration from a JSON file.
func LoadConfiguration(filename string) (*MfdConfig, error) {
	var err error
	configOnce.Do(func() {
		// Read the JSON file
		data, err := os.ReadFile(filename)
		if err != nil {
			return
		}

		// Unmarshal JSON into the configuration struct
		var config MfdConfig
		if err := json.Unmarshal(data, &config); err != nil {
			return
		}

		fixupConfigurationPaths(&config)
		configurationInstance = &config
	})
	return configurationInstance, err
}

func fixupConfigurationPaths(config *MfdConfig) {
	config.FilePath = strings.ReplaceAll(os.ExpandEnv(config.FilePath), "/", "\\")
	config.DcsSavedGamesPath = strings.ReplaceAll(os.ExpandEnv(config.FilePath), "/", "\\")
	config.DisplayConfigurationFile = strings.ReplaceAll(os.ExpandEnv(config.DisplayConfigurationFile), "/", "\\")
	config.Modules = strings.ReplaceAll(os.ExpandEnv(config.Modules), "/", "\\")
}

func (l *Logger) SetLogFile() {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.fileName = l.generateLogFileName()
	if l.file != nil {
		l.file.Close()
	}
	l.openLogFile()
}

func (l *Logger) generateLogFileName() string {
	currentTime := time.Now()
	return filepath.Join(getLogFolderPath(), "status_"+currentTime.Format("2006_01_02_15")+".log")
}

func getLogFolderPath() string {
	logFolderPath := filepath.Join(getSavedGamesFolder(), "MFDMF", "Logs")
	return logFolderPath
}

func getSavedGamesFolder() string {
	currentUser, err := user.Current()
	if err != nil {
		log.Fatalf("Failed to get current user: %v", err)
	}
	savedGamesFolder := filepath.Join(currentUser.HomeDir, "Saved Games")
	return savedGamesFolder
}

func (l *Logger) openLogFile() {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.fileName = l.generateLogFileName()

	logFolder := filepath.Dir(l.fileName)
	err := os.MkdirAll(logFolder, 0755)
	if err != nil {
		log.Fatalf("Failed to create log folder: %v", err)
	}

	file, err := os.OpenFile(l.fileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	l.file = file

	log.SetOutput(l.file)
}

func (l *Logger) Log(message string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	log.Println(message)
	fmt.Println(message)
}

var instance *Logger
var once sync.Once
var configurationInstance *MfdConfig
var configOnce sync.Once

func setDisplays(displays []Display) {
	for i := range displays {
		var configurator DisplayConfigurator = &displays[i] // Use a pointer to satisfy the interface
		err := configurator.ConfigureDisplay()
		if err != nil {
			fmt.Printf("Error configuring display %s: %v\n", displays[i].Name, err)
		}
	}
}

func GetLogger() *Logger {
	once.Do(func() {
		instance = &Logger{}
		instance.openLogFile()
	})
	return instance
}

// Loads the Display list from the specified filename
func readDisplaysJSON(filename string) ([]Display, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var displays []Display
	err = json.Unmarshal(data, &displays)
	if err != nil {
		return nil, err
	}
	return displays, nil
}

// Reads all of the modules from the specified path and below
func readModuleFiles(startingPath string) ([]Module, error) {
	var modules []Module

	// Walk the directory tree starting from the specified path
	err := filepath.Walk(startingPath, func(filePath string, fileInfo os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Check if the file is a JSON file
		if filepath.Ext(filePath) == ".json" {
			// Read the JSON file
			data, err := os.ReadFile(filePath)
			if err != nil {
				return err
			}

			// Unmarshal the JSON data into a wrapper structure with the "Modules" array
			var jsonData struct {
				Modules []Module `json:"modules"`
			}
			err = json.Unmarshal(data, &jsonData)
			if err != nil {
				return err
			}

			// Calculate the relative Category based on the starting path
			relativePath, err := filepath.Rel(startingPath, filePath)
			if err != nil {
				return err
			}

			// Set the Category for each module
			for i := range jsonData.Modules {
				jsonData.Modules[i].Category = strings.Replace(relativePath, ".json", "", 1)
			}

			// Append the modules from the wrapper to the main modules slice
			modules = append(modules, jsonData.Modules...)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return modules, nil
}

func setFullPathToFile(config *Configuration) {
	// Ensure config is not nil
	if config == nil {
		config = &Configuration{}
	}

	// Check if NeedsThrottleType is nil and initialize if necessary
	if config.NeedsThrottleType == nil {
		needsThrottleType := false
		config.NeedsThrottleType = &needsThrottleType
	}

	// Proceed with the rest of the function logic
	if !(config.FileName == "") {
		userPath := config.FileName

		if *config.NeedsThrottleType {
			replaceToken := "WH"
			if configurationInstance.UseCougar {
				replaceToken = "HC"
			}
			userPath = strings.ReplaceAll(userPath, "THROTTLE", replaceToken)
		}

		if !isPathInside(configurationInstance.FilePath, config.FileName) {
			userPath = path.Join(configurationInstance.FilePath, userPath)
		}

		fullPathToImage := strings.ReplaceAll(userPath, "/", "\\")
		config.FileName = fullPathToImage
	}
}

// Sets a Configuration equal to some of the Display values handles centering if required
func setConfigToDisplay(config *Configuration, display Display) {
	config.Display = &display
	config.NeedsThrottleType = display.NeedsThrottleType
	center := false
	if display.Center != nil {
		center = *display.Center
	}
	config.Center = &center

	useAsSwitch := false
	if display.UseAsSwitch != nil {
		useAsSwitch = *display.UseAsSwitch
	}
	config.UseAsSwitch = &useAsSwitch

	enabled := false
	if display.Enabled != nil {
		enabled = *display.Enabled
	}
	config.Enabled = &enabled

	if config.Left == nil && display.Left != nil {
		config.Left = display.Left
	}

	if config.Top == nil && display.Top != nil {
		config.Top = display.Top
	}

	if config.Width == nil && display.Width != nil {
		config.Width = display.Width
	}

	if config.Height == nil && display.Height != nil {
		config.Height = display.Height
	}

	copyPropertiesFromDisplay(config, display)
}

// Copy the Offset and Image handling properties from a Display to a Configuration
func copyPropertiesFromDisplay(config *Configuration, display Display) {
	if config.XOffsetStart == nil {
		config.XOffsetStart = display.XOffsetStart
	}
	if config.XOffsetFinish == nil {
		config.XOffsetFinish = display.XOffsetFinish
	}
	if config.YOffsetStart == nil {
		config.YOffsetStart = display.YOffsetStart
	}
	if config.YOffsetFinish == nil {
		config.YOffsetFinish = display.YOffsetFinish
	}
	if config.Opacity == nil {
		config.Opacity = display.Opacity
	}
	if config.Enabled == nil {
		config.Enabled = display.Enabled
	}
	if config.UseAsSwitch == nil {
		config.UseAsSwitch = display.UseAsSwitch
	}
	if config.Center == nil {
		config.Center = display.Center
	}
	if config.Left == nil {
		config.Left = display.Left
	}
	if config.Top == nil {
		config.Top = display.Top
	}
	if config.Width == nil {
		config.Width = display.Width
	}
	if config.Height == nil {
		config.Height = display.Height
	}
}

func isPathInside(parentPath, childPath string) bool {
	// Clean and normalize the paths
	parentPath = filepath.Clean(parentPath)
	childPath = filepath.Clean(childPath)

	// Ensure that the paths end with the path separator for accurate comparison
	parentPath += string(filepath.Separator)

	// Use strings.HasPrefix to check if childPath starts with parentPath
	return strings.HasPrefix(childPath, parentPath)
}

func ensurePathExists(path string) error {
	// Check if the path exists
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		// Create the directory and all necessary parents
		err = os.MkdirAll(path, os.ModePerm)
		if err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to check path: %w", err)
	}
	return nil
}

func setModuleFileName(module *Module) {
	var userPath = ""
	if module.FileName != "" {
		if !isPathInside(configurationInstance.FilePath, module.FileName) {
			userPath = path.Join(configurationInstance.FilePath, module.FileName)
		} else {
			userPath = module.FileName
		}
		module.FileName = strings.ReplaceAll(userPath, "/", "\\")
	}
}

func setConfigurationFileNames(config *Configuration) {
	setFullPathToFile(config)
	for i := range config.Configurations {
		conf := &config.Configurations[i]
		setFullPathToFile(conf)
		setFileNamesRecursive(conf)
	}
}

func setFileNamesRecursive(conf *Configuration) {
	if !isPathInside(configurationInstance.FilePath, conf.FileName) {
		setFullPathToFile(conf)
	}
	for i := range conf.Configurations {
		var subConfig = &conf.Configurations[i]
		if !isPathInside(configurationInstance.FilePath, subConfig.FileName) {
			setFileNamesRecursive(subConfig)
		}
	}
}

func setInitialValues(obj ConfigurationProvider) {
	// Initialize Dimensions fields if nil
	dims := obj.GetDimension()
	if dims.Left == nil {
		left := 0
		dims.Left = &left
	}
	if dims.Top == nil {
		top := 0
		dims.Top = &top
	}
	if dims.Width == nil {
		width := 0
		dims.Width = &width
	}
	if dims.Height == nil {
		height := 0
		dims.Height = &height
	}

	// Initialize Offsets fields if nil
	offsets := obj.GetOffset()
	if offsets.XOffsetStart == nil {
		xOffsetStart := 0
		offsets.XOffsetStart = &xOffsetStart
	}
	if offsets.XOffsetFinish == nil {
		xOffsetFinish := 0
		offsets.XOffsetFinish = &xOffsetFinish
	}
	if offsets.YOffsetStart == nil {
		yOffsetStart := 0
		offsets.YOffsetStart = &yOffsetStart
	}
	if offsets.YOffsetFinish == nil {
		yOffsetFinish := 0
		offsets.YOffsetFinish = &yOffsetFinish
	}

	// Initialize ImageProperties fields if nil
	imgProps := obj.GetImageProperties()
	if imgProps.Center == nil {
		center := false
		imgProps.Center = &center
	}
	if imgProps.Opacity == nil {
		opacity := float32(1.0)
		imgProps.Opacity = &opacity
	}
	if imgProps.Enabled == nil {
		enabled := true
		imgProps.Enabled = &enabled
	}
	if imgProps.UseAsSwitch == nil {
		useAsSwitch := false
		imgProps.UseAsSwitch = &useAsSwitch
	}
	if imgProps.NeedsThrottleType == nil {
		needsThrottleType := false
		imgProps.NeedsThrottleType = &needsThrottleType
	}
}

func enrichConfigurations(module *Module, displays *[]Display) {

	for i := range module.Configurations {
		config := &module.Configurations[i]
		config.Module = module
		// Set file name from module if not already set
		if config.FileName == "" {
			config.FileName = module.FileName
		}
		setConfigurationFileNames(config)
		enrichedConfig := enrichSingleConfig(config, displays)
		if strings.Contains(enrichedConfig.FileName, "THROTTLE") {
			replaceToken := "WH"
			if configurationInstance.UseCougar {
				replaceToken = "HC"
			}
			enrichedConfig.FileName = strings.ReplaceAll(enrichedConfig.FileName, "THROTTLE", replaceToken)
		}
		module.Configurations[i] = *enrichedConfig
		// Enrich sub-configurations recursively.
		enrichSubConfigs(config, displays)
	}
}

func enrichSingleConfig(config *Configuration, displays *[]Display) *Configuration {
	matched := false

	for _, display := range *displays {
		if strings.HasPrefix(config.Name, display.Name) {
			config.Display = &display

			// Copy properties from display to configuration.
			setConfigToDisplay(config, display)
			matched = true
			break
		}
	}

	// If no match is found, ensure default values.
	if !matched {
		var configurator ConfigurationProcessor = config // Use a pointer to satisfy the interface
		err := configurator.ConfigureConfiguration()
		if err != nil {
			fmt.Printf("Error configuring Configuration %s: %v\n", config.Name, err)
		}
		instance.Log(fmt.Sprintf("Configuration %s NOT matched", config.Name))
	}

	return config
}

func enrichSubConfigs(parentConfig *Configuration, displays *[]Display) {
	for i := range parentConfig.Configurations {
		subConfig := &parentConfig.Configurations[i]
		subConfig.Parent = parentConfig
		if subConfig.FileName == "" {
			subConfig.FileName = parentConfig.FileName
		}

		// Ensure initial values are set for each sub-configuration.
		setInitialValues(subConfig)

		// Enrich sub-configuration with display properties.
		enrichedSubConfig := enrichSingleConfig(subConfig, displays)
		enrichedSubConfig.Module = parentConfig.Module
		enrichedSubConfig.Display = parentConfig.Display

		// Recursively handle nested sub-configurations.
		enrichSubConfigs(enrichedSubConfig, displays)
		parentConfig.Configurations[i] = *enrichedSubConfig
	}
}

func indent(n int) string {
	return strings.Repeat("\t", n)
}

func formatConfiguration(module Module, config Configuration, level int) string {
	indentLevel := level + 1
	properties := fmt.Sprintf("%sName: %s\n", indent(indentLevel), config.Name)
	indentLevel++
	if config.Parent != nil && config.Parent.Name != "" {
		properties += fmt.Sprintf("%sParent: %s\n", indent(indentLevel), config.Parent.Name)
	} else {
		properties += fmt.Sprintf("%sParent Module: %s\n", indent(indentLevel), config.Module.Name)
	}
	properties += fmt.Sprintf("%sLeft: %d\n", indent(indentLevel), *config.Left)
	properties += fmt.Sprintf("%sTop: %d\n", indent(indentLevel), *config.Top)
	properties += fmt.Sprintf("%sWidth: %d\n", indent(indentLevel), *config.Width)
	properties += fmt.Sprintf("%sHeight: %d\n", indent(indentLevel), *config.Height)
	properties += fmt.Sprintf("%sXOffsetStart: %d\n", indent(indentLevel), *config.XOffsetStart)
	properties += fmt.Sprintf("%sXOffsetFinish: %d\n", indent(indentLevel), *config.XOffsetFinish)
	properties += fmt.Sprintf("%sYOffsetStart: %d\n", indent(indentLevel), *config.YOffsetStart)
	properties += fmt.Sprintf("%sYOffsetFinish: %d\n", indent(indentLevel), *config.YOffsetFinish)
	properties += fmt.Sprintf("%sCenter: %v\n", indent(indentLevel), *config.Center)
	properties += fmt.Sprintf("%sOpacity: %f\n", indent(indentLevel), *config.Opacity)
	properties += fmt.Sprintf("%sEnabled: %v\n", indent(indentLevel), *config.Enabled)
	properties += fmt.Sprintf("%sUseAsSwitch: %v\n", indent(indentLevel), *config.UseAsSwitch)
	var fileExists = false
	// Check if the file exists
	if _, err := os.Stat(config.FileName); err == nil {
		fileExists = true
	}
	properties += fmt.Sprintf("%sFileName: %s - Exists: %v\n", indent(indentLevel), config.FileName, fileExists)

	if len(config.Configurations) > 0 {
		properties += fmt.Sprintf("%sConfigurations:\n", indent(indentLevel))
		for _, subConfig := range config.Configurations {
			properties += formatConfiguration(module, subConfig, level+1)
		}
	}

	return properties
}

func formatModule(module *Module) string {
	result := fmt.Sprintf("Name: %s\n", module.Name)
	result += fmt.Sprintf("Display Name: %s\n", module.DisplayName)
	result += fmt.Sprintf("Category: %s\n", module.Category)
	result += fmt.Sprintf("FileName: %s\n", module.FileName)
	result += fmt.Sprintf("Tag: %s\n", module.Tag)

	for _, config := range module.Configurations {
		//setInitialValues(&config)
		result += formatConfiguration(*module, config, 1)
	}

	return result
}

func AddPrefixToFilename(filePath, prefix string) (string, error) {
	dir := filepath.Dir(filePath)       // Get the directory part of the path
	filename := filepath.Base(filePath) // Get the filename

	// Add the prefix to the filename
	newFilename := prefix + filename

	// Join the directory and new filename to get the modified path
	newPath := filepath.Join(dir, newFilename)

	return newPath, nil
}

func loadImageFile(fullPath string) (image.Image, error) {
	// Try loading as PNG or JPEG using gg.LoadImage
	img, err := gg.LoadImage(fullPath)
	if err != nil {
		// If loading as PNG or JPEG fails, try BMP using imaging.Open
		img, err = imaging.Open(fullPath)
		if err != nil {
			return nil, err
		}
	}
	return img, nil
}

func GetSaveDirectory(parentFileName string, moduleName string, rootConfigName string) string {
	if parentFileName == "" {
		return filepath.Join(getCacheBaseDirectory(), moduleName, rootConfigName)
	} else {
		return filepath.Dir(parentFileName)
	}
}

func applyOpacity(img image.Image, opacity float32) *image.RGBA {
	bounds := img.Bounds()
	rgbaImage := image.NewRGBA(bounds)

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()

			// Convert from 16-bit to 8-bit
			r8 := uint8(r >> 8)
			g8 := uint8(g >> 8)
			b8 := uint8(b >> 8)
			a8 := uint8(a >> 8)

			// Adjust alpha based on opacity
			adjustedAlpha := uint8(float32(a8) * opacity)

			// Premultiply color channels by adjusted alpha
			rPremultiplied := uint8(float32(r8) * float32(adjustedAlpha) / 255)
			gPremultiplied := uint8(float32(g8) * float32(adjustedAlpha) / 255)
			bPremultiplied := uint8(float32(b8) * float32(adjustedAlpha) / 255)

			rgbaImage.Set(x, y, color.RGBA{
				R: rPremultiplied,
				G: gPremultiplied,
				B: bPremultiplied,
				A: adjustedAlpha,
			})
		}
	}

	return rgbaImage
}

// Function to convert any image.Image to *image.RGBA
func convertToRGBA(src image.Image) *image.RGBA {
	if rgba, ok := src.(*image.RGBA); ok {
		return rgba
	}

	// Create a new RGBA image with the same bounds as the source
	bounds := src.Bounds()
	rgba := image.NewRGBA(bounds)

	// Draw the source image onto the RGBA image
	draw.Draw(rgba, bounds, src, bounds.Min, draw.Src)
	return rgba
}

func drawAxesWithTicks(img image.Image, xaxisColor color.Color, yaxisColor color.Color, drawTicks bool, tickLength int, tickInterval int, tickColor color.Color, textColor color.Color, numberLeftToRight bool) image.Image {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// Convert the input image to RGBA to allow modifications
	rgbaImg := convertToRGBA(img)

	centerX := width / 2
	centerY := height / 2

	// Draw the Y axis (vertical line)
	for y := 0; y < height; y++ {
		rgbaImg.Set(centerX, y, yaxisColor)
	}

	// Draw the X axis (horizontal line)
	for x := 0; x < width; x++ {
		rgbaImg.Set(x, centerY, xaxisColor)
	}

	if drawTicks && tickInterval > 0 {
		drawer := &font.Drawer{
			Dst:  rgbaImg,
			Src:  image.NewUniform(textColor),
			Face: basicfont.Face7x13,
		}

		// Draw tick marks and labels along the X-axis
		for x := centerX; x < width; x += tickInterval {
			for y := -tickLength / 2; y <= tickLength/2; y++ {
				rgbaImg.Set(x, centerY+y, tickColor)
			}
			label := x - centerX
			if numberLeftToRight {
				label = x
			}
			drawer.Dot = fixed.Point26_6{X: fixed.I(x), Y: fixed.I(centerY + tickLength + 10)}
			drawer.DrawString(fmt.Sprintf("%d", label))
		}
		for x := centerX - tickInterval; x >= 0; x -= tickInterval {
			for y := -tickLength / 2; y <= tickLength/2; y++ {
				rgbaImg.Set(x, centerY+y, tickColor)
			}
			label := x - centerX
			if numberLeftToRight {
				label = x
			}
			drawer.Dot = fixed.Point26_6{X: fixed.I(x), Y: fixed.I(centerY + tickLength + 10)}
			drawer.DrawString(fmt.Sprintf("%d", label))
		}

		// Draw tick marks and labels along the Y-axis
		for y := centerY; y < height; y += tickInterval {
			for x := -tickLength / 2; x <= tickLength/2; x++ {
				rgbaImg.Set(centerX+x, y, tickColor)
			}
			label := -(y - centerY)
			if numberLeftToRight {
				label = y
			}
			drawer.Dot = fixed.Point26_6{X: fixed.I(centerX + tickLength + 5), Y: fixed.I(y + drawer.Face.Metrics().Ascent.Ceil()/2)}
			drawer.DrawString(fmt.Sprintf("%d", label))
		}
		for y := centerY - tickInterval; y >= 0; y -= tickInterval {
			for x := -tickLength / 2; x <= tickLength/2; x++ {
				rgbaImg.Set(centerX+x, y, tickColor)
			}
			label := -(y - centerY)
			if numberLeftToRight {
				label = y
			}
			textWidth := drawer.MeasureString(fmt.Sprintf("%d", label)).Ceil()
			drawer.Dot = fixed.Point26_6{X: fixed.I(centerX - textWidth - 5), Y: fixed.I(y + drawer.Face.Metrics().Ascent.Ceil()/2)}
			drawer.DrawString(fmt.Sprintf("%d", label))
		}
	}

	return rgbaImg
}

func createRectangle(minX, minY, maxX, maxY int) image.Rectangle {
	return image.Rect(minX, minY, maxX, maxY)
}

func getCacheBaseDirectory() string {
	return filepath.Join(getSavedGamesFolder(), "MFDMF", "Cache")
}

func clearCacheFolder() {
	cacheFolder := getCacheBaseDirectory()
	removeContents(cacheFolder)
	instance.Log(fmt.Sprintf("The cache has been cleared at %s", cacheFolder))
}

func removeContents(path string) error {
	// Open the directory
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()

	// Get a list of all files and subdirectories in the directory
	entries, err := dir.Readdir(-1)
	if err != nil {
		return err
	}

	// Loop through each entry and remove it, handling subdirectories recursively
	for _, entry := range entries {
		entryPath := filepath.Join(path, entry.Name())
		if entry.IsDir() {
			// If it's a subdirectory, call removeContents recursively
			err := removeContents(entryPath)
			if err != nil {
				return err
			}
			// Remove the empty directory
			err = os.Remove(entryPath)
			if err != nil {
				return err
			}
		} else {
			// If it's a file, remove the file
			err := os.Remove(entryPath)
			if err != nil {
				return err
			}
		}
	}

	os.Remove(path)
	return nil
}

func saveImageAsJPGAndPNG(saveImagePath string, img image.Image) error {
	fileName := fmt.Sprintf("%s.jpg", saveImagePath)
	instance.Log(fmt.Sprintf("Saving %s", fileName))
	jpgFile, err := os.Create(fileName)
	if err != nil {
		return err
	}
	defer jpgFile.Close()
	err = jpeg.Encode(jpgFile, img, &jpeg.Options{Quality: 80})
	if err != nil {
		return err
	}
	/*
		pngFile, err := os.Create(fmt.Sprintf("%s.png", saveImagePath))
		if err != nil {
			return err
		}
		defer pngFile.Close()
	*/
	//return png.Encode(pngFile, img)
	return jpeg.Encode(jpgFile, img, &jpeg.Options{Quality: 80})
}

// buildConfigToFileMap recursively builds a dictionary mapping Configuration.Name to its file name
func buildConfigToFileMap(config Configuration, rootPath string, configToFileMap map[string]string) {
	// Generate the file path for this configuration
	filePath := filepath.Join(getCacheBaseDirectory(), config.Module.Name, rootPath)
	ensurePathExists(filePath)
	filePath = filepath.Join(filePath, config.Name)
	configToFileMap[config.Name] = filePath

	// Recursively process sub-configurations
	for _, subConfig := range config.Configurations {
		buildConfigToFileMap(subConfig, rootPath, configToFileMap)
	}
}

// generateConfigToFileMap processes all configurations in a module and generates the dictionary
func generateConfigToFileMap(module Module) map[string]string {
	configToFileMap := make(map[string]string)

	// Process each top-level configuration
	for _, config := range module.Configurations {
		buildConfigToFileMap(config, config.Name, configToFileMap)
	}

	return configToFileMap
}

func ConvertNRGBAToRGBAUsingDraw(src *image.NRGBA) *image.RGBA {
	bounds := src.Bounds()
	dst := image.NewRGBA(bounds)

	// Use draw.Draw to copy pixels from NRGBA to RGBA
	draw.Draw(dst, bounds, src, bounds.Min, draw.Src)

	return dst
}

// centerImageWithCropAndResize centers a resized child image onto a resized parent image.
// If childImgPath is blank or nil, only the parent image is cropped, resized, and saved.
func (config *Configuration) CenterImageWithCropAndResize(subConfigIndex int) error {
	var configurator ConfigurationProcessor = config // Use a pointer to satisfy the interface
	parentImgPath := config.FileName
	// Open the parent image
	parentFile, err := os.Open(parentImgPath)
	if err != nil {
		return fmt.Errorf("failed to open parent image: %v", err)
	}
	defer parentFile.Close()

	parentImg, _, err := image.Decode(parentFile)
	if err != nil {
		return fmt.Errorf("failed to decode parent image: %v", err)
	}
	cropRectParent := configurator.GetCropRect()
	parentSize := configurator.GetSize()

	// Crop and resize the parent image
	croppedParentImg := cropImage(parentImg, cropRectParent)
	resizedParentImg := imaging.Resize(croppedParentImg, parentSize.X, parentSize.Y, imaging.Lanczos)
	outputFileName := configToFiles[config.Name]
	config.Image = (*image.RGBA)(resizedParentImg)

	if configurationInstance.SaveCroppedImages {
		saveImage(outputFileName+"-crop", resizedParentImg)
	}

	// If childImgPath is blank or nil, save only the resized parent image
	if subConfigIndex == -1 {
		outputImg := convertToRGBA(resizedParentImg)
		if configurationInstance.ShowRulers {
			outputImg = convertToRGBA(drawAxesWithTicks(outputImg, RedColor, RedColor, true, 10, configurationInstance.RulerSize, BlackColor, BlackColor, true))
		}
		return saveImage(outputFileName, outputImg)
	}

	subConfig := &config.Configurations[subConfigIndex]
	childImgPath := subConfig.FileName
	// Open the child image
	childFile, err := os.Open(childImgPath)
	if err != nil {
		return fmt.Errorf("failed to open child image: %v", err)
	}
	defer childFile.Close()

	childImg, _, err := image.Decode(childFile)
	if err != nil {
		return fmt.Errorf("failed to decode child image: %v", err)
	}

	var subConfigurator ConfigurationProcessor = subConfig
	cropRectChild := subConfigurator.GetCropRect()
	childSize := subConfigurator.GetSize()

	// Crop and resize the child image
	croppedChildImg := cropImage(childImg, cropRectChild)
	resizedChildImg := imaging.Resize(croppedChildImg, childSize.X, childSize.Y, imaging.Lanczos)
	outputFileName = configToFiles[subConfig.Name]
	subConfig.Image = (*image.RGBA)(resizedChildImg)
	if configurationInstance.SaveCroppedImages {
		saveImage(outputFileName+"-crop", resizedChildImg)
	}

	// Get dimensions of both resized images
	parentBounds := resizedParentImg.Bounds()
	childBounds := resizedChildImg.Bounds()

	parentWidth := parentBounds.Dx()
	parentHeight := parentBounds.Dy()
	childWidth := childBounds.Dx()
	childHeight := childBounds.Dy()

	// Calculate the position to center the child image on the parent image
	offsetX := (parentWidth - childWidth) / 2
	offsetY := (parentHeight - childHeight) / 2

	// Create a new RGBA canvas with the size of the resized parent image
	outputImg := image.NewRGBA(parentBounds)

	// Draw the resized parent image onto the canvas
	draw.Draw(outputImg, parentBounds, resizedParentImg, image.Point{}, draw.Src)

	// Draw the resized child image onto the canvas at the calculated position
	draw.Draw(outputImg, childBounds.Add(image.Point{X: offsetX, Y: offsetY}), resizedChildImg, image.Point{}, draw.Over)

	// Add axes and ticks using drawAxesWithTicks if ShowRulers is true
	if configurationInstance.ShowRulers {
		outputImg = convertToRGBA(drawAxesWithTicks(outputImg, RedColor, RedColor, true, 10, configurationInstance.RulerSize, BlackColor, BlackColor, true))
	}

	// Save the resulting composite image
	return saveImage(outputFileName, outputImg)
}

// cropImage crops an input image to the specified rectangle.
func cropImage(src image.Image, rect image.Rectangle) image.Image {
	cropped := imaging.Crop(src, rect)
	return cropped
}

// saveImage saves an image to a file based on its extension (.png or .jpg).
func saveImage(fileName string, img image.Image) error {
	outputFile, err := os.Create(fileName + ".jpg")
	if err != nil {
		return fmt.Errorf("failed to create output file: %v", err)
	}
	defer outputFile.Close()

	err = jpeg.Encode(outputFile, img, &jpeg.Options{Quality: 90})
	if err != nil {
		return fmt.Errorf("failed to save output file: %v", err)
	}

	return nil
}

func processConfiguration(config *Configuration, subIndex int) error {
	var configurator ConfigurationProcessor = config
	configurator.CenterImageWithCropAndResize(subIndex)

	// Process sub-configurations recursively
	for i := range config.Configurations {
		configurator.CenterImageWithCropAndResize(i)
	}
	return nil
}

var configToFiles map[string]string

func processModule(module *Module, displays []Display) error {
	instance.Log(fmt.Sprintf("Processing Module %s", module.DisplayName))
	// Set the Filename to the fullpath if it's not in the module filePath
	setModuleFileName(module)
	// Enrich all the Configurations and Sub-Configurations with Display data
	enrichConfigurations(module, &displays)
	configToFiles = generateConfigToFileMap(*module)
	// process each Configuration of the Module
	for _, config := range module.Configurations {

		err := processConfiguration(&config, -1)
		if err != nil {
			return fmt.Errorf("error processing the configuration %s: %w", config.Name, err)
		}
	}
	instance.Log(fmt.Sprintf("BEGIN ********** %s//%s *********", module.Category, module.Name))
	moduleInfo := formatModule(module)
	instance.Log(moduleInfo)
	instance.Log(fmt.Sprintf("END ********** %s//%s *********", module.Category, module.Name))
	return nil
}

var (
	module     string
	subModule  string
	clearCache bool
)

func init() {
	flag.StringVar(&module, "mod", "", "Module to select")
	flag.StringVar(&subModule, "sub", "", "Sub-Module to select")
	flag.BoolVar(&clearCache, "clear", false, "Clears the cache")
}

func main() {
	flag.Parse()

	logger := GetLogger()
	logger.Log("Starting GOMFD!")

	if clearCache {
		clearCacheFolder()
		return
	}

	currentUser, err := user.Current()
	if err != nil {
		fmt.Println("Error getting the current User", err)
		return
	}

	configFilePath := currentUser.HomeDir + "\\Saved Games\\MFDMF\\appsettings.json"
	currentConfig, err := LoadConfiguration(configFilePath)
	if err != nil {
		fmt.Println("Error reading Configuration", err)
		return
	}
	displayJsonPath := currentConfig.DisplayConfigurationFile

	// Read displays.json file
	displays, err := readDisplaysJSON(displayJsonPath)
	if err != nil {
		fmt.Println("Error reading displays.json:", err)
		return
	}

	// Make sure all the values are set
	setDisplays(displays)

	// Load the modules
	modulesPath := currentConfig.Modules
	modules, err := readModuleFiles(modulesPath)
	if err != nil {
		fmt.Println("Error reading JSON files:", err)
		return
	}

	// Process each module
	counter := 0
	for _, module := range modules {
		err := processModule(&module, displays)
		if err != nil {
			fmt.Printf("Error processing module %s, Error %s", module.Name, err)
			return
		}
		counter++
	}
	instance.Log(fmt.Sprintf("Finished processing %d modules", counter))
}
