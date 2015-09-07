
# go-ssh-check
**Configuration Check with Go**

This application can be used to connect to multiple virtual machines concurrently and to perform certain tasks on them, e.g.. checking configuration, inspecting files.

At the moment, there are only two types of inspection:
- Check if a file exists on the server
- Check if a file contains a certain text block.

**Usage:**

The worker apps must be installed and run, in order to perform the tasks.

Start a new ssh-check process:

```go run sshcheck <config.json> <output.json>```

Output json file is optional. Default file to be created is "result.json".  

Connect to a running ssh-check process (without initialization, no removal of previous data) 

```go run sshcheck -s```

Start a worker app for processing tasks: 

```go run sshcheck_worker```

Refer to the example config.json file for the structure of json-configuration file.

**Details:**

There are two separate apps to be used in this application.

**sshcheck.go: configurer & job-starter**
- initializes the postgres database
- parses config.json file and loads the data to the database.
- tracks the current status of the tasks
- writes a json file when the task finishes

**sshcheck_worker.go:**
- processes the the tasks given in the json-config file.
- allows multiple instances

Both applications require a common postgre-sql database for communication and task-handling.

The application might require some individual tuning:

**CONFIG FOR MAIN-APP (sshcheck.go)**
- db_url: Required. Your postgres server url.
- timeout_period_for_ssh: The amount of time that a locked task to be available again for other workers, in case it's not processed.  
- default_output: default output json file (default: result.json)
- progress_refresh_interval: Refresh interval for monitoring the progress.

**CONFIG FOR WORKER-APP (sshcheck_worker.go)**
- db_url: Required. Your postgres server url.
- number_of_rows_to_be_processed: Number of rows that will be retrieved from the database as bulk import. A higher value allows faster processing. A lower value makes more precise monitoring in the main-app. (Default: 6)
- number_of_workers: Number of allowed concurrent ssh-calls from a worker app. Higher is faster, and lower is safer in the connections, especially when there are lots of checks for a single machine. (Default: 3)
- check_interval: time period for monitoring new data in the database. This is not relevant when the process is already started, a task-set runs without a pause (default value: 5 sec. This means the worker apps begin working at most 5 seconds after the main-app initializes content)

**Config-Json:**
- ssh-user: username for the ssh connection
- private-key-file: openssh private key
- server: list of servers or remote vm's to be accessed for the ssh-check

**Example config.json:**
```
{
   "ssh-user":"ec2-user",
   "private-key-file":"/id_rsa",
   "server":[
   "52.48.19.12",
   "51.73.16.180",
   "52.58.19.432",
   "54.63.16.180"
   ],
   "private-key-file":"/home/ec2-user/.ssh/id_rsa",
   "check_config_file_exists":[
      {
         "name":"test1",
         "path":"/home/ec2-user/file1"
      },
      {
         "name":"test2",
         "path":"/home/ec2-user/file2"
      }
   ],
   "check_config_file_contains":[
      {
         "name":"test3",
         "path":"/home/ec2-user/file1",
         "check":"word1"
      },
      {
         "name":"test4",
         "path":"/home/ec2-user/file1",
         "check":"word2"
      }
   ]
}
```

**Screenshot**

![screenshot](https://raw.githubusercontent.com/mdireskeneli/go-ssh-check/master/screenshot.png)

**Some implementation details**

The worker application can be run as multiple instances on different machines. Its main task is to process the input data and write to the result-table in the postgres-db. The workers await the new task-set, when they are started.
When the tasks are available, they are marked in the tasks-table as status:OPEN.
If a row is currently being processed by a worker app, then it is marked with status:LOCKED.

If a row cannot be processed by a certain worker app (ie. when the app fails or crushes, or when the connection cannot be established) then this row will return to the status:OPEN and thus will be available to other workers after a given time (default: 1min).
