# DB-Admin

This tool allows you to inspect, split, and merge db-files. 
It is useful if you have more than one conode, or if you want to migrate only
 part of the conode's DB over to a new conode.
The tool doesn't rely on knowledge of any particular service, but it rather 
uses the underlying db structure as imposed by onet.

## Inspect

The first thing you can do is to list all data stored in a db in a concise 
manner:

```bash
dbadmin inspect xxxx.db
```

By default, it prints the list of services stored, and how much space they take.
You can get more information by using the `-v` flag:

```bash
dbadmin inspect -v xxxx.db
```

## Store Service

In order to migrate data from one service to a new conode, you first need to 
store the data:

```bash
dbadmin store xxxx.db ServiceName
```

This will output all data of the given service to a file called `ServiceName
.db`.

## Merge Service

Merging a service file into an existing DB overwrites the data of this 
service with the data given in the file:

```bash
dbadmin merge yyyy.db ServiceName.db
```

This will take all the data stored in `ServiceName.db` and use it to 
overwrite the data of the service in `yyyy.db`.
