# go-ssh-check
Configuration Check with Go

Usage:

go run go-ssh-check <config.json> <output.json>

Example Config:

{
   "ssh-user":"ec2-user",
   "server":"remote-server-ip",
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
