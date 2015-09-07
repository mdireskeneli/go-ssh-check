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

// CONFIG
const timeout_period_for_ssh int32 = int32(60)                                             // 60 seconds
const db_url string = "postgres://sshcheck:sshcheck@psql_host_ip/sshcheck?sslmode=disable" // ?sslmode=verify-full
const default_output string = "result.json"
const progress_refresh_interval time.Duration = time.Second

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

type ResultJson struct {
	ResultEntry []ResultEntry `json:"result"`
}

type ResultEntry struct {
	id           int    `json:"id"`
	Name         string `json:"name"`
	Server       string `json:"server_ip"`
	TaskType     string `json:"task_type"`
	Result       bool   `json:"result"`
	ErrorMessage string `json:"error_message"`
}

func main() {

	timeStart := time.Now()
	db, err := sql.Open("postgres", "postgres://sshcheck:sshcheck@54.93.96.180/sshcheck?sslmode=disable") // ?sslmode=verify-full
	checkErr(err)
	defer db.Close()

	if len(os.Args) < 2 {
		logging.Fatal("\nUsage: go run go-ssh-check <config.json> <output.json>")
	}
	inputFile := os.Args[1]
	var totalNumberOfJobs int = 0
	if inputFile != "-s" {
		totalNumberOfJobs = setup(inputFile, db)
	}

	fmt.Printf("RESULT: \n")
	var taskExists bool = true
	for taskExists {
		printJobNum(db, totalNumberOfJobs)
		taskExists = checkIfTaskExists(db)
		time.Sleep(progress_refresh_interval)
	}

	elapsed := time.Since(timeStart)
	fmt.Printf("\n Total time: %+v", elapsed)

	// write to json

	rows, err := db.Query("select * from Result")
	checkErr(err)

	resultEntryList := make([]ResultEntry, 0)

	for rows.Next() {
		resultEntry := new(ResultEntry)
		err := rows.Scan(&resultEntry.id, &resultEntry.Name, &resultEntry.Server, &resultEntry.TaskType, &resultEntry.Result, &resultEntry.ErrorMessage)
		checkErr(err)
		resultEntryList = append(resultEntryList, *resultEntry)
	}
	rows.Close()

	resultJson := new(ResultJson)
	resultJson.ResultEntry = resultEntryList

	writeResultToJsonFile(*resultJson)
}

func setup(inputFile string, db *sql.DB) int {
	configFile, err := os.Open(inputFile)
	if err != nil {
		logging.Fatal("opening config file" + err.Error())
	}
	jsonParser := json.NewDecoder(configFile)
	configElement := new(ConfigElement)
	if err = jsonParser.Decode(&configElement); err != nil {
		logging.Fatal("parsing config file" + err.Error())
	}

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
	db.Exec("CREATE TABLE Task(id serial, taskname text, tasktype text, filepath text, checkstr text, ip text, status text, num_trial int, task_start_time int);") //int , serial

	db.Exec("DROP TABLE Jobinfo;")
	//	db.Exec("CREATE TABLE Jobinfo(processed_job_num int);")
	//	db.Exec("insert into Jobinfo(processed_job_num) values (0);")

	db.Exec("DROP TABLE result;")
	_, st := db.Exec("CREATE TABLE result(id serial, taskname text, server_ip text, tasktype text, result_val boolean, error_message text);") //int , serial

	if st != nil {
		logging.Fatal("error writing to db" + err.Error())
	}

	fmt.Println("# Inserting values")

	txn, err := db.Begin()
	checkErr(err)
	stmt, err := txn.Prepare(pq.CopyIn("task", "taskname", "tasktype", "filepath", "checkstr", "ip", "status", "num_trial", "task_start_time"))
	checkErr(err)

	totalNumberOfJobs := (len(configElement.CheckFileContains) + len(configElement.CheckFileExists)) * len(configElement.Server)

	fmt.Printf("CONFIGURATION:\n")
	for _, host_ip := range configElement.Server {
		// performing the check for each server
		fmt.Printf("Server: " + host_ip + "\n")
		for _, v := range configElement.CheckFileContains {
			_, err = stmt.Exec(v.Name, "file_contains", v.Path, v.Check, host_ip, "OPEN", 0, 0)
			if err != nil {
				log.Fatal(err)
			}
		}
		for _, v := range configElement.CheckFileExists {
			_, err = stmt.Exec(v.Name, "file_exists", v.Path, "", host_ip, "OPEN", 0, 0)
			if err != nil {
				log.Fatal(err)
			}
		}
	}

	_, err = stmt.Exec()
	checkErr(err)
	err = stmt.Close()
	checkErr(err)
	err = txn.Commit()
	checkErr(err)
	return totalNumberOfJobs
}

func checkIfTaskExists(db *sql.DB) bool {
	rows, err := db.Query("select * from Task where num_trial < 3")
	checkErr(err)
	taskExists := rows.Next()
	rows.Close()
	db.Exec("update Task set status='OPEN' where status='LOCKED' and task_start_time < $1", int32(time.Now().Unix())-timeout_period_for_ssh)
	return taskExists
}

func printJobNum(db *sql.DB, totalNumberOfJobs int) {
	var jobNum int = 0
	err := db.QueryRow("select count(*) from result").Scan(&jobNum)
	checkErr(err)
	fmt.Print("\r")
	fmt.Printf("Number of Jobs processed: %v", jobNum)
	if totalNumberOfJobs != 0 {
		fmt.Printf("/%v", totalNumberOfJobs)
	}
}

func checkErr(err error) {
	if err != nil {
		log.Printf("%T %+v", err, err)
	}
}

func writeResultToJsonFile(result ResultJson) {
	var outputFile string
	if len(os.Args) < 3 {
		outputFile = default_output
	} else {
		outputFile = os.Args[2]
	}
	fp, err := os.Create(outputFile)
	if err != nil {
		logging.Fatal("Unable to create %v. Err: %v.", outputFile, err)
	}
	defer fp.Close()
	encoder := json.NewEncoder(fp)
	if err = encoder.Encode(result); err != nil {
		logging.Fatal("Unable to encode Json file. Err: %v.", err)
	}
}
