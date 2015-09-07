package main

// main package
import (
	//"./lib"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/koding/logging"
	"github.com/lib/pq"
	//_ "github.com/lib/pq"
	"log"
	"os"
	"time"
)

type ConfigElement struct {
	User              string   `json:"ssh-user"`
	Server            []string `json:"server"`
	Key               string   `json:"private-key-file"`
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
	Server string `json:"server"`
}

type CheckFileExistsResult struct {
	Name   string `json:"name"`
	Result bool   `json:"result"`
	Server string `json:"server"`
}

func main() {
	timeStart := time.Now()

	if len(os.Args) < 2 {
		logging.Fatal("\nUsage: go run go-ssh-check <config.json> <output.json>")
	}
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

	db, err := sql.Open("postgres", "postgres://sshcheck:sshcheck@54.93.96.180/sshcheck?sslmode=disable") // ?sslmode=verify-full
	if err != nil {
		logging.Fatal("error writing to db" + err.Error())
	}
	defer db.Close()

	_, err = db.Exec("DROP TABLE Config;")
	_, err = db.Exec("CREATE TABLE Config(id serial, username text, private_key text);")
	if err != nil {
		logging.Fatal("error writing to db" + err.Error())
	} //int , serial
	_, err = db.Exec("Insert into Config(username, private_key) values ($1, $2);", configElement.User, configElement.Key) //int , serial
	if err != nil {
		logging.Fatal("error writing to db" + err.Error())
	}

	db.Exec("DROP TABLE TASK;")
	db.Exec("CREATE TABLE Task(id serial, taskname text, tasktype text, filepath text, checkstr text, ip text, status text, num_trial int);") //int , serial

	db.Exec("DROP TABLE Jobinfo;")
	db.Exec("CREATE TABLE Jobinfo(processed_job_num int);")
	db.Exec("insert into Jobinfo(processed_job_num) values (0);")

	db.Exec("DROP TABLE result;")
	_, st := db.Exec("CREATE TABLE result(id serial, taskname text, server_ip text, tasktype text, result_val boolean, error_message text);") //int , serial

	if st != nil {
		logging.Fatal("error writing to db" + err.Error())
	}

	fmt.Println("# Inserting values")

	txn, err := db.Begin()
	if err != nil {
		log.Fatal(err)
	}

	stmt, err := txn.Prepare(pq.CopyIn("task", "taskname", "tasktype", "filepath", "checkstr", "ip", "status", "num_trial"))
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("CONFIGURATION:\n")
	for _, host_ip := range configElement.Server {
		// performing the check for each server
		fmt.Printf("Server: " + host_ip + "\n")
		for _, v := range configElement.CheckFileContains {
			_, err = stmt.Exec(v.Name, "file_contains", v.Path, v.Check, host_ip, "OPEN", 0)
			if err != nil {
				log.Fatal(err)
			}
		}
		for _, v := range configElement.CheckFileExists {
			_, err = stmt.Exec(v.Name, "file_exists", v.Path, "", host_ip, "OPEN", 0)
			if err != nil {
				log.Fatal(err)
			}
		}
	}

	_, err = stmt.Exec()
	if err != nil {
		log.Fatal(err)
	}

	err = stmt.Close()
	if err != nil {
		log.Fatal(err)
	}

	err = txn.Commit()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("RESULT:\n")

	elapsed := time.Since(timeStart)

	fmt.Printf("%+v\n", elapsed)

}
