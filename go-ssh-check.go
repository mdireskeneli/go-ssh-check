package main

// main package
import (
	"encoding/json"
	"fmt"
	"github.com/koding/logging"
	"github.com/weekface/easyssh"
	"os"
)

const OUTPUT_FILE string = "out.json"

type ConfigElement struct {
	User              string `json:"ssh-user"`
	Server            string `json:"server"`
	Key               string `json:"private-key-file"`
	CheckFileContains []struct {
		Name  string `json:"name"`
		Path  string `json:"path"`
		Check string `json:"check"`
	} `json:"check_config_file_contains"`
	CheckFileExists []struct {
		Name string `json:"name"`
		Path string `json:"path"`
	} `json:"check_config_file_exists"`
}

type ConfigReturn struct {
	CheckFileContainsResult []CheckFileContainsResult `json:"check_config_file_contains_result"`
	CheckFileExistsResult   []CheckFileExistsResult   `json:"check_config_file_exists_result"`
}

type CheckFileContainsResult struct {
	Name   string `json:"name"`
	Result bool   `json:"result"`
}

type CheckFileExistsResult struct {
	Name   string `json:"name"`
	Result bool   `json:"result"`
}

func main() {
	inputFile := os.Args[1]
	configFile, err := os.Open(inputFile)
	if err != nil {
		logging.Fatal("opening config file" + err.Error())
	}
	jsonParser := json.NewDecoder(configFile)
	configElement := new(ConfigElement)
	if err = jsonParser.Decode(&configElement); err != nil {
		logging.Fatal("parsing config file" + err.Error())
	}
	fmt.Printf("CONFIGURATION:\n")
	fmt.Printf("%+v\n", configElement)
	sContains := make([]CheckFileContainsResult, 0)
	sExists := make([]CheckFileExistsResult, 0)

	checkContainsCount := len(configElement.CheckFileContains)
	checkExistsCount := len(configElement.CheckFileExists)

	channelContains := make(chan CheckFileContainsResult, checkContainsCount)
	channelExists := make(chan CheckFileExistsResult, checkExistsCount)

	fmt.Printf("STARTING TESTS:\n")
	for _, v := range configElement.CheckFileContains {
		fmt.Printf("%+v\n", v)
		go checkFileContains(v.Name, v.Path, v.Check, channelContains, *configElement)
	}

	for _, v := range configElement.CheckFileExists {
		fmt.Printf("%+v\n", v)
		go checkFileExists(v.Name, v.Path, channelExists, *configElement)
	}

	for i := 0; i < checkContainsCount; i++ {
		result := <-channelContains
		sContains = append(sContains, result)
	}

	for i := 0; i < checkExistsCount; i++ {
		result := <-channelExists
		sExists = append(sExists, result)
	}

	fmt.Printf("RESULT:\n")
	fmt.Printf("%+v\n", sContains)
	fmt.Printf("%+v\n", sExists)

	resultTest := new(ConfigReturn)
	resultTest.CheckFileContainsResult = sContains
	resultTest.CheckFileExistsResult = sExists

	writeResultToJsonFile(*resultTest)
}

func writeResultToJsonFile(result ConfigReturn) {
	outputFile := os.Args[2]
	fp, err := os.Create(outputFile)
	if err != nil {
		logging.Fatal("Unable to create %v. Err: %v.", OUTPUT_FILE, err)
	}
	defer fp.Close()
	encoder := json.NewEncoder(fp)
	if err = encoder.Encode(result); err != nil {
		logging.Fatal("Unable to encode Json file. Err: %v.", err)
	}
}

func checkFileContains(name string, path string, check string, channelContains chan CheckFileContainsResult, configElement ConfigElement) {
	logging.Debug("Checking FileContains: " + "Path: " + path + "Check: " + check)
	result := new(CheckFileContainsResult)
	result.Name = name

	ssh := getSsh(configElement)
	// command: grep -q "something" file; [ $? -eq 0 ] && echo "yes" || echo "no"
	response, err := ssh.Run("grep -q \"" + check + "\" " + path + "; [ $? -eq 0 ] && echo \"1\" || echo \"0\"")
	if err != nil {
		logging.Error("error connecting server" + err.Error())
	} else {
		result.Result = getResultFromResponse(response)
		channelContains <- *result
	}
}

func getSsh(configElement ConfigElement) easyssh.MakeConfig {
	ssh := easyssh.MakeConfig{
		User:   configElement.User,
		Server: configElement.Server,
		Key:    configElement.Key,
		//Port: "22",
	}
	return ssh
}

func checkFileExists(name string, path string, channelExists chan CheckFileExistsResult, configElement ConfigElement) {
	logging.Debug("Checking FileExists: " + "Path: " + path)
	result := new(CheckFileExistsResult)
	result.Name = name

	ssh := getSsh(configElement)
	//command: [ -f file ] && echo "yes" || echo "no"
	response, err := ssh.Run("[ -f " + path + " ] && echo \"1\" || echo \"0\"")
	if err != nil {
		logging.Error("error connecting server" + err.Error())
	} else {
		result.Result = getResultFromResponse(response)
		channelExists <- *result
	}
}

func getResultFromResponse(response string) bool {
	if response[0:1] == "1" {
		return true
	} else if response[0:1] == "0" {
		return false
	}
	return false
}
