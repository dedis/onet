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

## Backup service

You can backup the data of a service like this:

```bash
dbadmin extract --source xxxx.db --destination backup.db ServiceName.*
```

This will output all data of the given service to a file called `backup.db`.
The servicename(s) given as arguments can contain regular expressions.
A service like `skipchain` has at least 3 buckets:
- `Skipchain`
- `Skipchain_skipblocks`
- `Skipchainversion`

The `dbadmin` tool doesn't know about any relationship.
It is up to the caller to make sure that all buckets are copied correctly.
Onet usually puts all databases belonging to a service in buckets that all
 start with the name of the service, followed by `_` and the name of the
  additional bucket required by the service.

## Restore Service

Merging a service file into an existing DB copies the data of this 
service to the destination DB.
If the destination DB or the service in the destination DB already exists
, the call must include `--overwrite`

```bash
dbadmin extxract --source backup.db --destination xxxx.db
```

This will take all the data stored in `back.db` and use it to 
overwrite the data of the service in `yyyy.db`.

## Copy Service From One Conode to Another

If you have two conodes with databases `xxxx.db` and `yyyy.db`, you can copy
 the services from one conode to another with the following command:
 
```bash
dbadmin extract --source xxxx.db --destination yyyy.db --overwrite
```

This will copy _all_ services from `xxxx.d` to `yyyy.db`. If you only want to
 copy one service, you can do:
 
```bash
dbadmin extract --source xxxx.db --destination yyyy.db --overwrite ServiceName.*
```
