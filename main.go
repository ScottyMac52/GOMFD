package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/disintegration/imaging"
	"github.com/fogleman/gg"
	"github.com/nfnt/resize"
)

type Rectangle struct {
	Left   int `json:"left,omitempty"`
	Top    int `json:"top,omitempty"`
	Width  int `json:"width,omitempty"`
	Height int `json:"height,omitempty"`
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

func (o *Outer) GetDimension() *Properties {
    if o.isSet() {
        return &o.Properties
    }
    if o.Inner != nil {
        return o.Inner.GetDimension()
    }
    return nil
}

// LoadConfig loads the configuration from a JSON file.
func LoadConfiguration(filename string) *MfdConfig {
	configOnce.Do(func() {
		// Read the JSON file
		data, err := os.ReadFile(filename)
		if err != nil {
			panic(err)
		}

		// Unmarshal JSON into the configuration struct
		var config MfdConfig
		if err := json.Unmarshal(data, &config); err != nil {
			panic(err)
		}
		fixupConfigurationPaths(&config)
		configurationInstance = &config
	})
	return configurationInstance
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
}

var instance *Logger
var once sync.Once
var configurationInstance *MfdConfig
var configOnce sync.Once

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
				jsonData.Modules[i].Category = relativePath
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

// Normalizes the file path for the image to MfdConfig.FilePath
func setFullPathToFile(config *Configuration) {
	if !(config.FileName == "") {
		userPath := config.FileName
		if config.NeedsThrottleType {
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

// Gets the Recentangle from the Left, Top, Width and Height of the Configuration
func getRectangleFromConfiguration(config *Configuration) Rectangle {
	rect := Rectangle{Left: config.Left, Top: config.Top, Width: config.Width, Height: config.Height}
	return rect
}

// Sets the coordinates for the Configuration to the specified Rectangle
func setConfigurationSizeFromRectangle(rect *Rectangle, config *Configuration) {
	config.Left = rect.Left
	config.Top = rect.Top
	config.Width = rect.Width
	config.Height = rect.Height
}

// Gets the Recentangle from the Left, Top, Width and Height of the Display
func getRectangleFromDisplay(display Display) Rectangle {
	rect := Rectangle{Left: display.Left, Top: display.Top, Width: display.Width, Height: display.Height}
	return rect
}

// Sets a Configuration equal to some of the Display values handles centering if required
func setConfigToDisplay(config *Configuration, display Display) {
	config.Display = &display
	config.NeedsThrottleType = display.NeedsThrottleType
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
	copyPropertiesFromDisplay(config, display)
}

// Copy the Offset and Image handling properties from a Display to a Configuration
func copyPropertiesFromDisplay(config *Configuration, display Display) {
	if config.XOffsetStart == 0 {
		config.XOffsetStart = display.XOffsetStart
	}
	if config.XOffsetFinish == 0 {
		config.XOffsetFinish = display.XOffsetFinish
	}
	if config.YOffsetStart == 0 {
		config.YOffsetStart = display.YOffsetStart
	}
	if config.YOffsetFinish == 0 {
		config.YOffsetFinish = display.YOffsetFinish
	}
	if config.Opacity == 0 {
		config.Opacity = display.Opacity
	}
	if !config.Enabled {
		config.Enabled = display.Enabled
	}
	if !config.UseAsSwitch {
		config.UseAsSwitch = display.UseAsSwitch
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

var currentTrail string

func enrichConfiguration(module *Module, config *Configuration, displays []Display) {
	config.Module = module
	if config.FileName == "" {
		config.FileName = module.FileName
	}

	currentTrail = fmt.Sprintf("%s-%s-", module.Name, config.Name)
	// Enrich the main configuration
	enrichSingleConfig(config, displays)

	if len(config.Configurations) == 0 {
		fmt.Printf(currentTrail)
	}

	// Enrich sub-configurations
	enrichSubConfigs(config, displays)
	fmt.Printf(currentTrail)
}

func enrichSubConfigs(parentConfig *Configuration, displays []Display) {
	for i := range parentConfig.Configurations {
		subConfig := &parentConfig.Configurations[i]
		subConfig.Parent = parentConfig
		if subConfig.FileName == "" {
			subConfig.FileName = parentConfig.FileName
		}

		enrichSingleConfig(subConfig, displays)
		if len(subConfig.Configurations) == 0 {
			// end of the line!
			fmt.Printf(currentTrail)
			currentTrail = ""
		}
		enrichSubConfigs(subConfig, displays) // Recursive call to handle nested sub-configurations
	}
}

func enrichSingleConfig(config *Configuration, displays []Display) {
	matched := false
	currentTrail += fmt.Sprintf("%s-", config.Name)
	for _, display := range displays {
		if strings.HasPrefix(config.Name, display.Name) {
			config.Display = &display
			setConfigToDisplay(config, display)
			matched = true
			break // Break as soon as a match is found
		}
	}
	if !matched {
		config.Display = nil
		if config.Opacity == 0 {
			config.Opacity = 1.0
		}
		if !config.Enabled {
			config.Enabled = true
		}
		if !config.UseAsSwitch {
			config.UseAsSwitch = false
		}
	}
}

func indent(n int) string {
	return strings.Repeat("\t", n)
}

func formatConfiguration(config Configuration, level int) string {
	indentLevel := level + 1
	properties := fmt.Sprintf("%sName: %s\n", indent(indentLevel), config.Name)
	indentLevel++
	properties += fmt.Sprintf("%sLeft: %v\n", indent(indentLevel), config.Left)
	properties += fmt.Sprintf("%sTop: %v\n", indent(indentLevel), config.Top)
	properties += fmt.Sprintf("%sWidth: %d\n", indent(indentLevel), config.Width)
	properties += fmt.Sprintf("%sHeight: %d\n", indent(indentLevel), config.Height)
	properties += fmt.Sprintf("%sXOffsetStart: %d\n", indent(indentLevel), config.XOffsetStart)
	properties += fmt.Sprintf("%sXOffsetFinish: %d\n", indent(indentLevel), config.XOffsetFinish)
	properties += fmt.Sprintf("%sYOffsetStart: %d\n", indent(indentLevel), config.YOffsetStart)
	properties += fmt.Sprintf("%sYOffsetFinish: %d\n", indent(indentLevel), config.YOffsetFinish)
	properties += fmt.Sprintf("%sCenter: %v\n", indent(indentLevel), config.Center)
	properties += fmt.Sprintf("%sOpacity: %f\n", indent(indentLevel), config.Opacity)
	properties += fmt.Sprintf("%sEnabled: %v\n", indent(indentLevel), config.Enabled)
	properties += fmt.Sprintf("%sUseAsSwitch: %v\n", indent(indentLevel), config.UseAsSwitch)
	var fileExists = false
	// Check if the file exists
	if _, err := os.Stat(config.FileName); err == nil {
		fileExists = true
	}
	properties += fmt.Sprintf("%sFileName: %s - Exists: %v\n", indent(indentLevel), config.FileName, fileExists)

	if len(config.Configurations) > 0 {
		properties += fmt.Sprintf("%sConfigurations:\n", indent(indentLevel))
		for _, subConfig := range config.Configurations {
			properties += formatConfiguration(subConfig, level+2)
		}
	}

	return properties
}

func formatModule(module Module) string {
	result := fmt.Sprintf("Name: %s\n", module.Name)

	for _, config := range module.Configurations {
		result += formatConfiguration(config, 1)
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

func structToString(input interface{}) string {
	valueOf := reflect.ValueOf(input)
	if valueOf.Kind() != reflect.Struct {
		return ""
	}

	var result string
	typ := valueOf.Type()

	for i := 0; i < valueOf.NumField(); i++ {
		field := valueOf.Field(i)
		fieldName := typ.Field(i).Name
		fieldValue := field.Interface()

		result += fmt.Sprintf("%s: %v\n", fieldName, fieldValue)
	}

	return result
}

func structToJSON(input interface{}) (string, error) {
	jsonData, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	return string(jsonData), nil
}

func loadImage(config *Configuration) (image.Image, error) {
	fullPath := config.FileName
	if config.NeedsThrottleType {

		var throttleType string

		if configurationInstance.UseCougar {
			throttleType = "HC"
		} else {
			throttleType = "WH"
		}

		fullPath = strings.Replace(fullPath, "THROTTLE", throttleType, 1)
		config.FileName = fullPath
	}
	return loadImageFile(fullPath)
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

func cropImage(src image.Image, x, y, width, height int) image.Image {
	rect := image.Rect(x, y, x+width, y+height)
	cropped := image.NewNRGBA(rect)
	draw.Draw(cropped, cropped.Bounds(), src, image.Point{x, y}, draw.Src)
	return cropped
}

func logCachePath(fullPath string) string {
	modifiedString := strings.Replace(fullPath, getCacheBaseDirectroy(), "", -1)
	return modifiedString
}

func createCompositeImage(module *Module, rootConfig Configuration, currentConfig Configuration, parentFileName string) {
	message := fmt.Sprintf("%s-%s-%s-%s\n", module.Name, rootConfig.Name, currentConfig.Name, logCachePath(parentFileName))
	instance.Log(message)
	fmt.Printf(message)
	var saveImagePath = ""
	//var printMessage = ""
	canvas := image.NewRGBA(image.Rect(0, 0, rootConfig.Width, rootConfig.Height))
	//var relativeCachePath = ""
	//var parentRelativeCachePath = logCachePath(parentFileName)

	// If there is no parent, we are starting a composite to save
	if parentFileName == "" {
		saveImagePath = filepath.Join(getCacheBaseDirectroy(), module.Name, rootConfig.Name)
		//	relativeCachePath = logCachePath(saveImagePath)
		//	printMessage = fmt.Sprintf("Starting %s\n", relativeCachePath)
		//	instance.Log(printMessage)
		//	fmt.Printf(printMessage)
	} else {
		//	printMessage = fmt.Sprintf("\tContinuing %s\n", parentRelativeCachePath)
		//	instance.Log(printMessage)
		//	fmt.Printf(printMessage)
		saveImagePath = filepath.Dir(parentFileName)
		parentImage, err := loadImageFile(parentFileName)
		if err != nil {
			fmt.Println("Error loading image:", err)
			return
		}
		draw.Draw(canvas, canvas.Bounds(), parentImage, image.Point{}, draw.Over)
	}

	// Load the image for the current configuration and draw it on the canvas
	imageForCurrentConfig, err := loadImage(&currentConfig)
	if err != nil {
		fmt.Println("Error loading image:", err)
		return
	}
	//instance.Log(fmt.Sprintf("\t\tDrawing %s onto %s", logCachePath(currentConfig.Name), parentRelativeCachePath))

	// Resize the superimposed image to currentConfig.Width and currentConfig.Height
	// Crop a rectangle from the image based on currentConfig.Left, currentConfig.Top,
	// currentConfig.Width, and currentConfig.Height
	offsetWidth := currentConfig.XOffsetFinish - currentConfig.XOffsetStart
	offsetHeight := currentConfig.YOffsetFinish - currentConfig.YOffsetStart

	croppedImage := cropImage(imageForCurrentConfig, currentConfig.XOffsetStart, currentConfig.YOffsetStart, offsetWidth, offsetHeight)

	// Resize the cropped image to currentConfig.Width and currentConfig.Height
	croppedImage = resize.Resize(uint(currentConfig.Width), uint(currentConfig.Height), croppedImage, resize.Lanczos3)
	//printMessage = fmt.Sprintf("\t\t\tResizing %s onto %s as (%d,%d)\n", logCachePath(currentConfig.FileName), logCachePath(parentRelativeCachePath), currentConfig.Width, currentConfig.Height)
	//instance.Log(printMessage)
	//fmt.Printf(printMessage)

	// Calculate the draw position based on currentConfig.Left and currentConfig.Top
	drawPosition := image.Point{}
	if rootConfig.Name == currentConfig.Name {
		drawPosition = image.Point{X: 0, Y: 0}
	} else {
		// Check if the Center property is true for the current configuration
		if currentConfig.Center {
			// Calculate the center position within the parent image
			parentBounds := canvas.Bounds()
			centerX := (parentBounds.Dx() - croppedImage.Bounds().Dx()) / 2
			centerY := (parentBounds.Dy() - croppedImage.Bounds().Dy()) / 2
			drawPosition = image.Point{X: centerX, Y: centerY}
		} else {
			drawPosition = image.Point{X: currentConfig.Left, Y: currentConfig.Top}
		}
	}

	// Convert Opacity from percentage to float32 between 0.0 and 1.0
	opacity := float32(currentConfig.Opacity)

	// Create a new RGBA image with the same dimensions as the croppedImage
	rgbaImage := image.NewRGBA(croppedImage.Bounds())

	// Apply opacity to each pixel in the cropped image.
	for y := croppedImage.Bounds().Min.Y; y < croppedImage.Bounds().Max.Y; y++ {
		for x := croppedImage.Bounds().Min.X; x < croppedImage.Bounds().Max.X; x++ {
			r, g, b, a := croppedImage.At(x, y).RGBA()

			// Calculate adjusted alpha separately.
			adjustedAlpha := uint8(float32(a>>8) * opacity)

			// Calculate color channels without modifying the alpha channel.
			r8 := uint8(uint32(r>>8) * uint32(adjustedAlpha) / 0xff)
			g8 := uint8(uint32(g>>8) * uint32(adjustedAlpha) / 0xff)
			b8 := uint8(uint32(b>>8) * uint32(adjustedAlpha) / 0xff)

			rgbaImage.Set(x, y, color.RGBA{
				R: r8,
				G: g8,
				B: b8,
				A: adjustedAlpha,
			})
		}
	}

	os.MkdirAll(saveImagePath, os.ModeAppend)

	if configurationInstance.SaveCroppedImages {
		croppedFileName := rootConfig.Name + "_" + currentConfig.Name + "_CROP.png"
		originalFileName := rootConfig.Name + "_" + currentConfig.Name + "_ORIG.png"
		if parentFileName == "" {
			croppedFileName = rootConfig.Name + ".png"
		}
		croppedFullPath := filepath.Join(saveImagePath, croppedFileName)
		originalFullPath := filepath.Join(saveImagePath, originalFileName)
		saveImage(croppedImage, croppedFullPath)
		saveImage(imageForCurrentConfig, originalFullPath)
	}

	// Draw the resized and cropped image with opacity onto the canvas at the specified position
	draw.Draw(canvas, image.Rectangle{Min: drawPosition, Max: drawPosition.Add(croppedImage.Bounds().Size())}, rgbaImage, image.Point{}, draw.Over)

	fileName := currentConfig.Name + ".png"
	currentCompositePath := filepath.Join(saveImagePath, fileName)
	relativeCompositePath := logCachePath(currentCompositePath)

	if configurationInstance.ShowRulers {
		// Specify a custom color (e.g., green)
		customColor := color.RGBA{255, 0, 0, 255}
		err := drawRuler(canvas, configurationInstance.RulerSize, 10, customColor)
		if err != nil {
			instance.Log(fmt.Sprintf("Unable to draw ruler on %s", relativeCompositePath))
		}
	}

	// Save the composite image to a file
	message = fmt.Sprintf("\tSaving %s\n", relativeCompositePath)
	instance.Log(message)
	fmt.Printf(message)
	saveImage(canvas, currentCompositePath)

	// Recursively process sub-configurations
	for _, subConfig := range currentConfig.Configurations {
		createCompositeImage(module, rootConfig, subConfig, currentCompositePath)
	}
}

func drawRuler(targetImage *image.RGBA, rulerSize, interval int, rulerColor color.RGBA) error {
	// Calculate the center of the image
	centerX := targetImage.Bounds().Dx() / 2
	centerY := targetImage.Bounds().Dy() / 2

	//instance.Log(fmt.Sprintf("\t\t\tDrawing X/Y axis ruler every %d px, with center at (%d,%d) and bounds of (%d,%d)", rulerSize, centerX, centerY, targetImage.Bounds().Dx(), targetImage.Bounds().Dy()))

	// Draw the vertical ruler line
	draw.Draw(targetImage, image.Rect(centerX, 0, centerX+1, targetImage.Bounds().Dy()), &image.Uniform{rulerColor}, image.Point{}, draw.Over)

	// Draw the horizontal ruler line
	draw.Draw(targetImage, image.Rect(0, centerY, targetImage.Bounds().Dx(), centerY+1), &image.Uniform{rulerColor}, image.Point{}, draw.Over)

	// Draw tick marks along the X-axis
	for x := centerX - rulerSize; x <= centerX+rulerSize; x++ {
		if x%interval == 0 {
			y1 := centerY - 5
			y2 := centerY + 6
			for y := y1; y <= y2; y++ {
				targetImage.Set(x, y, rulerColor)
			}
		}
	}

	// Draw tick marks along the Y-axis
	for y := centerY - rulerSize; y <= centerY+rulerSize; y++ {
		if y%interval == 0 {
			x1 := centerX - 5
			x2 := centerX + 6
			for x := x1; x <= x2; x++ {
				targetImage.Set(x, y, rulerColor)
			}
		}
	}

	return nil
}

func saveImage(img image.Image, filePath string) error {
	outputFile, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer outputFile.Close()

	// Encode the image as PNG and save it
	if err := png.Encode(outputFile, img); err != nil {
		return err
	}

	return nil
}

func getCacheBaseDirectroy() string {
	return filepath.Join(getSavedGamesFolder(), "MFDMF", "Cache")
}

func clearCacheFolder() string {
	cacheFolder := getCacheBaseDirectroy()
	removeContents(cacheFolder)
	return fmt.Sprintf("The cache has been cleared at %s", cacheFolder)
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

func printConfigurations(moduleName string, configs []Configuration, level int) {
	for _, config := range configs {
		fmt.Printf("%sConfiguration: %s-%s\n", getIndentation(level), moduleName, config.Name)
		printSubConfigDefs(config.Configurations, level+1)
	}
}

func printSubConfigDefs(subConfigDefs []Configuration, level int) {
	for _, subConfigDef := range subConfigDefs {
		fmt.Printf("%sSubConfigDef: %s-%s\n", getIndentation(level), subConfigDef.Parent, subConfigDef.Name)
		// Recursively print sub-configurations if present
		if len(subConfigDef.Configurations) > 0 {
			fmt.Printf("%sSub-Configurations:\n", getIndentation(level))
			printSubConfigDefs(subConfigDef.Configurations, level+1)
		}
	}
}

func getIndentation(level int) string {
	indent := ""
	for i := 0; i < level; i++ {
		indent += "\t"
	}
	return indent
}

func loadJSONData(moduleFilePath string, displays []Display) (*Module, error) {
	// Read the module file
	moduleData, err := os.ReadFile(moduleFilePath)
	if err != nil {
		return nil, err
	}

	// Unmarshal module data
	var module Module
	err = json.Unmarshal(moduleData, &module)
	if err != nil {
		return nil, err
	}

	// Map the displays to configurations and set Module and Parent pointers
	for i := range module.Configurations {
		module.Configurations[i].Module = &module // Set the Module pointer

		// If the configuration has sub-configurations (assuming such a structure exists)
		for j := range module.Configurations[i].Configurations {
			module.Configurations[i].Configurations[j].Parent = &module.Configurations[i] // Set the Parent pointer
		}

		for _, display := range displays {
			if strings.HasPrefix(module.Configurations[i].Name, display.Name) {
				module.Configurations[i].Display = &display // Set the Display pointer
				break
			}
		}
	}

	return &module, nil
}

var (
	module     string
	subModule  string
	verbose    bool
	clearCache bool
)

func init() {
	flag.StringVar(&module, "mod", "", "Module to select")
	flag.StringVar(&subModule, "sub", "", "Sub-Module to select")
	flag.BoolVar(&verbose, "verbose", false, "Enable verbose mode")
	flag.BoolVar(&clearCache, "clear", false, "Clears the cache")
}

func main() {
	flag.Parse()

	if verbose {
		fmt.Println("Verbose mode is enabled.")
	}

	logger := GetLogger()
	logger.Log("Starting GOMFD!")


	if clearCache {
		statusMessage := clearCacheFolder()
		instance.Log(statusMessage)
		fmt.Println(statusMessage)
		return
	}

	currentUser, err := user.Current()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	configFilePath := currentUser.HomeDir + "\\Saved Games\\MFDMF\\appsettings.json"
	currentConfig := LoadConfiguration(configFilePath)
	displayJsonPath := currentConfig.DisplayConfigurationFile

	// Read displays.json file
	displays, err := readDisplaysJSON(displayJsonPath)
	if err != nil {
		fmt.Println("Error reading displays.json:", err)
		return
	}

	// Load the modules
	modulesPath := currentConfig.Modules
	modules, err := readModuleFiles(modulesPath)
	if err != nil {
		fmt.Println("Error reading JSON files:", err)
		return
	}

	// Enrich Configuration properties
	for _, module := range modules {
		setModuleFileName(&module)
		for i := range module.Configurations {
			var config = &module.Configurations[i]
			config.Module = &module
			config.Parent = nil
			// enrich Configurations with Displays as required
			enrichConfiguration(&module, config, displays)

			// Fix up all the paths from relative to absolute
			setConfigurationFileNames(config)

			// Print out the configurations for the module
			printConfigurations(module.Name, module.Configurations, 1)

			// Create the images
			//createCompositeImage(&module, *config, *config, "")
		}
	}
}
