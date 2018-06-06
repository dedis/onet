Navigation: [DEDIS](https://github.com/dedis/doc/tree/master/README.md) ::
[Onet](README.md) ::
Database Backup and Recovery

# Database Backup and Recovery

Users of ONet have the option to make use of its built-in database.

We use [bbolt](https://github.com/coreos/bbolt), which supports "fully
serializable ACID transactions" to ensure data integrity for ONet users. Users
should be able to do the following:

- Backup data while ONet is running
- Recovery from a backup in case of data corruption

# Backup
Users are recommend to perform frequent backups such that data can be recovered
if ONet nodes fail. ONet stores all of its data in the context folder, specified
by `$CONODE_SERVICE_PATH`. If unset, it defaults to
- `~/Library/conode/Services` on macOS,
- `$HOME\AppData\Local\Conode` on Windows, or
- `~/.local/share/conode` on other Unix/Linux.

Hence, to backup, it is recommended to use a standard backup tool, such as
rsync, to copy the folder to a different physical location periodically.
The database keeps a transaction log.
So performing backups in the middle of a transaction should not be a problem.
However, it is still recommended to check the data integrity of the backed-up file
using the bbolt CLI, i.e. `bolt check database_name.db`.

To install the bbolt CLI, see [Bolt Installation](https://github.com/coreos/bbolt#installing).

# Recovery

A data corruption is easy to detect as ONet nodes would crash when reading from
a corrupted database, at startup or during operation. Concretely, the bbolt
library would panic, e.g.
[here](https://github.com/coreos/bbolt/blob/386b851495d42c4e02908838373a06d0a533e170/freelist.go#L237).
This behavior is produced by writing a few blocks on random data using `dd` to
the database.

In case of data corruption, the database must be restored from a backup by
simply copying the backup copy to the context directory, and then starting the
conode again. It is the user's responsibility to make sure that the data is up
to date, e.g. by reading the latest data from running ONet nodes.

# Interacting with the database

The primary and recommended methods to interact with the database is
[`Load`](https://godoc.org/github.com/dedis/onet#Context.Load) and
[`Save`](https://godoc.org/github.com/dedis/onet#Context.Save). If more control
on the database is needed, then we can ask the context to return a database
handler and bucket name using the function
[`GetAdditionalBucket`](https://godoc.org/github.com/dedis/onet#Context.GetAdditionalBucket).
All the [bbolt functions](https://godoc.org/github.com/coreos/bbolt) can be used
with the database handler. However, the user should avoid creating new buckets
using the bbolt functions and only use `GetAdditionalBucket` to avoid bucket
name conflicts.
