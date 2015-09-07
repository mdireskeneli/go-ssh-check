package main

import (
	"database/sql"
	"fmt"
	"github.com/koding/logging"
	"github.com/lib/pq"
	"github.com/weekface/easyssh"
	"log"
	"strconv"
	"time"
)

const number_of_rows_to_be_processed int = 10
const number_of_workers int = 4

type TaskEntry struct {
	id        int
	taskname  string
	tasktype  string
	filepath  string
	checkstr  string
	ip        string
	status    string
	num_trial int
}

type ResultEntry struct {
	id           int
	Name         string
	Server       string
	TaskType     string
	Result       bool
	ErrorMessage string
}

var (
	username    string
	private_key string
)

const select_task_query string = "select * from Task where status = $1 and num_trial < 3 order by id asc for update"
const db_url string = "postgres://sshcheck:sshcheck@54.93.96.180/sshcheck?sslmode=disable" // ?sslmode=verify-full

func main() {
	db, err := sql.Open("postgres", db_url)
	checkErr(err)
	defer db.Close()

	for {
		taskExists := checkIfTaskExists(db)
		if taskExists {
			for checkIfTaskExists(db) {
				startMainJob(db)
			}
		}
		time.Sleep(5 * time.Second)
	}
}

func startMainJob(db *sql.DB) {
	fmt.Println("\n # Reading values")

	err := db.QueryRow("select username, private_key from config;").Scan(&username, &private_key)
	checkErr(err)

	txn, err := db.Begin()
	checkErr(err)

	rows, err := txn.Query(select_task_query, "OPEN")
	checkErr(err)

	defer rows.Close()

	taskEntryList := make([]TaskEntry, 0)
	resultEntryList := make([]ResultEntry, 0)

	var i int

	for i = 0; rows.Next() && i < number_of_rows_to_be_processed; i++ {
		taskEntry := new(TaskEntry)
		err := rows.Scan(&taskEntry.id, &taskEntry.taskname, &taskEntry.tasktype, &taskEntry.filepath, &taskEntry.checkstr, &taskEntry.ip, &taskEntry.status, &taskEntry.num_trial)
		checkErr(err)
		taskEntryList = append(taskEntryList, *taskEntry)
		log.Println(taskEntry.id, taskEntry.taskname)
	}
	var bulkSize = i

	txn.Exec("update task set status = 'LOCKED' where id in (" + getIdTaskListForInQuery(taskEntryList) + ");")

	txn.Commit()

	taskEntryChannel := make(chan TaskEntry, bulkSize)
	resultEntryChannel := make(chan ResultEntry, bulkSize)

	for w := 1; w <= number_of_workers; w++ {
		go worker(w, taskEntryChannel, resultEntryChannel)
	}

	for _, taskEntry := range taskEntryList {
		taskEntryChannel <- taskEntry
	}
	close(taskEntryChannel)

	for i := 0; i < bulkSize; i++ {
		resultEntry := <-resultEntryChannel
		//fmt.Print("\n" + "Result: ")
		//fmt.Print(resultEntry)
		resultEntryList = append(resultEntryList, resultEntry)
	}

	taskIdList := getIdTaskListForInQuery(taskEntryList)
	resultIdList := getIdResultListForInQuery(resultEntryList)
	//processed_job_num := len(resultIdList)

	txn, err = db.Begin()

	_, err = txn.Exec("delete from result where id in (" + resultIdList + ");") // clear list in case of previous entires with errors
	checkErr(err)

	checkErr(err)
	stmt, err := txn.Prepare(pq.CopyIn("result", "taskname", "server_ip", "tasktype", "result_val", "error_message"))
	checkErr(err)

	for _, r := range resultEntryList {
		_, err = stmt.Exec(r.Name, r.Server, r.TaskType, r.Result, r.ErrorMessage)
		checkErr(err)
	}

	_, err = stmt.Exec()
	checkErr(err)
	err = stmt.Close()
	checkErr(err)

	_, err = txn.Exec("delete from task where id in (" + resultIdList + ");") // parametrized variables do not work here
	checkErr(err)

	_, err = txn.Exec("update task set status = 'OPEN', num_trial = num_trial+1 where id in (" + taskIdList + ")") // parametrized variables do not work here
	checkErr(err)

	//	txn.Exec("update jobinfo set processed_job_num = processed_job_num + " + strconv.Itoa(processed_job_num) + " ;")

	txn.Commit()
}

func getIdTaskListForInQuery(taskEntryList []TaskEntry) string {
	inQuery := ""
	for _, taskEn := range taskEntryList {
		if inQuery != "" {
			inQuery += ", "
		}
		inQuery += strconv.Itoa(taskEn.id)
	}
	return inQuery
}

func getIdResultListForInQuery(resultEntryList []ResultEntry) string {
	inQuery := ""
	for _, resEn := range resultEntryList {
		if inQuery != "" {
			inQuery += ", "
		}
		inQuery += strconv.Itoa(resEn.id)
	}
	return inQuery
}

func checkFileContains(taskEntry TaskEntry, resultEntry chan ResultEntry) {
	logging.Debug("Checking FileContains: " + "Path: " + taskEntry.filepath + "Check: " + taskEntry.checkstr)
	result := new(ResultEntry)
	result.id = taskEntry.id
	result.Name = taskEntry.taskname
	result.Server = taskEntry.ip
	result.TaskType = taskEntry.tasktype

	ssh := getSsh(taskEntry.ip)
	// command: grep -q "something" file; [ $? -eq 0 ] && echo "yes" || echo "no"
	response, err := ssh.Run("grep -q \"" + taskEntry.checkstr + "\" " + taskEntry.filepath + "; [ $? -eq 0 ] && echo \"1\" || echo \"0\"")
	if err != nil {
		logging.Error("error connecting server: " + err.Error())
		result.ErrorMessage = err.Error()
	} else {
		result.Result = getResultFromResponse(response)
	}
	resultEntry <- *result

}

func checkFileExists(taskEntry TaskEntry, resultEntry chan ResultEntry) {
	logging.Debug("Checking FileExists: " + "Path: " + taskEntry.filepath)
	result := new(ResultEntry)
	result.id = taskEntry.id
	result.Name = taskEntry.taskname
	result.Server = taskEntry.ip
	result.TaskType = taskEntry.tasktype

	ssh := getSsh(taskEntry.ip)
	//command: [ -f file ] && echo "yes" || echo "no"
	response, err := ssh.Run("[ -f " + taskEntry.filepath + " ] && echo \"1\" || echo \"0\"")
	if err != nil {
		logging.Error("error connecting server: " + err.Error())
		result.ErrorMessage = err.Error()
	} else {
		result.Result = getResultFromResponse(response)
	}
	resultEntry <- *result
}

func getResultFromResponse(response string) bool {
	if response[0:1] == "1" {
		return true
	} else if response[0:1] == "0" {
		return false
	}
	return false
}

func getSsh(host_ip string) easyssh.MakeConfig {
	ssh := easyssh.MakeConfig{
		User:   username,
		Server: host_ip,
		Key:    private_key,
		//Port: "22",
	}
	return ssh
}

func worker(id int, taskEntryChannel <-chan TaskEntry, resultEntryChannel chan ResultEntry) {
	for taskEntry := range taskEntryChannel {
		fmt.Print("\nPerforming task: ")
		fmt.Print(taskEntry.id)
		if taskEntry.tasktype == "file_exists" {
			checkFileExists(taskEntry, resultEntryChannel)
		} else if taskEntry.tasktype == "file_contains" {
			checkFileContains(taskEntry, resultEntryChannel)
		} else {
			fmt.Print("Unknown tasktype: " + taskEntry.tasktype)
			resultEntryChannel <- *new(ResultEntry)
		}
	}

}

func checkIfTaskExists(db *sql.DB) bool {
	rows, err := db.Query(select_task_query, "OPEN")
	checkErr(err)

	taskExists := rows.Next()
	return taskExists
}

func checkErr(err error) {
	if err != nil {
		log.Printf("%T %+v", err, err)
	}
}
