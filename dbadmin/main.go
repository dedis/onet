package main

import (
	"os"
	"regexp"

	"go.etcd.io/bbolt"

	"github.com/urfave/cli"
	"go.dedis.ch/onet/v3/log"
	"golang.org/x/xerrors"
)

func main() {
	cliApp := cli.NewApp()
	cliApp.Name = "dbadmin"
	cliApp.Usage = "work with onet service dbs"
	cliApp.Version = "0.1"
	cliApp.Commands = cli.Commands{
		{
			Name:      "inspect",
			Usage:     "print information about an existing db",
			Action:    inspect,
			ArgsUsage: "source.db",
			Flags: cli.FlagsByName{
				cli.StringFlag{
					Name:      "source,src",
					Usage:     "Indicate source database",
					Required:  false,
					TakesFile: true,
				},
				cli.BoolFlag{
					Name:  "verbose,v",
					Usage: "print additional information",
				},
			},
		},
		{
			Name:      "extract",
			Usage:     "extract information from one db",
			Action:    extract,
			ArgsUsage: "service1 [service2 [service3...]]",
			Flags: cli.FlagsByName{
				cli.StringFlag{
					Name:      "source,src",
					Usage:     "Indicate source database",
					Required:  true,
					TakesFile: true,
				},
				cli.StringFlag{
					Name: "destination,dst",
					Usage: "Indicate destination database - must not" +
						" exist yet",
					Required:  true,
					TakesFile: true,
				},
				cli.BoolFlag{
					Name: "overwrite",
					Usage: "allow overwriting of existing dbs and" +
						" buckets",
					Required: false,
				},
			},
		},
	}
	cliApp.Flags = []cli.Flag{
		cli.IntFlag{
			Name:  "debug, d",
			Value: 0,
			Usage: "debug-level: 1 for terse, 5 for maximal",
		},
	}
	cliApp.Before = func(c *cli.Context) error {
		log.SetDebugVisible(c.Int("debug"))
		return nil
	}

	err := cliApp.Run(os.Args)
	if err != nil {
		log.Fatalf("Error while running app: %+v", err)
	}
}

func inspect(c *cli.Context) error {
	var dbName string
	if c.NArg() != 1 {
		if c.String("source") == "" {
			return xerrors.New("Please give the following arguments: source.db")
		}
		dbName = c.String("source")
	} else {
		dbName = c.Args().First()
	}
	verbose := c.Bool("verbose")
	if _, err := os.Stat(dbName); os.IsNotExist(err) {
		return xerrors.New("this db doesn't exist")
	}
	db, err := bbolt.Open(dbName, 0600, nil)
	if err != nil {
		return xerrors.Errorf("opening db: %v", err)
	}

	log.Info("Opened", dbName)

	err = db.View(func(tx *bbolt.Tx) error {
		return tx.ForEach(func(name []byte, b *bbolt.Bucket) error {
			log.Info("Bucket:", string(name))
			if verbose {
				stats := b.Stats()
				log.Infof("Has %d entries using %d bytes", stats.KeyN,
					stats.LeafAlloc)
			}
			return nil
		})
	})
	if err != nil {
		return xerrors.Errorf("couldn't list buckets: %+v", err)
	}

	return nil
}

func extract(c *cli.Context) error {
	var buckets []*regexp.Regexp
	if c.NArg() == 0 {
		log.Info("No service given - copying all services")
		buckets = append(buckets, regexp.MustCompile(".*"))
	} else {
		for _, b := range c.Args() {
			buckets = append(buckets, regexp.MustCompile(b))
		}
	}

	overwrite := c.Bool("overwrite")
	srcName := c.String("source")
	dstName := c.String("destination")
	if _, err := os.Stat(srcName); err != nil {
		return xerrors.Errorf("cannot read '%s': %v", srcName, err)
	}
	if _, err := os.Stat(dstName); err == nil && !overwrite {
		return xerrors.Errorf("destination '%s' will be created, "+
			"please remove it first", dstName)
	}

	dbSrc, err := bbolt.Open(srcName, 0600, nil)
	if err != nil {
		return xerrors.Errorf("couldn't open source-DB: %v", err)
	}
	dbDst, err := bbolt.Open(dstName, 0600, nil)
	if err != nil {
		return xerrors.Errorf("couldn't open destination-DB: %v", err)
	}

	err = dbSrc.View(func(txSrc *bbolt.Tx) error {
		return dbDst.Update(func(txDest *bbolt.Tx) error {
			return txSrc.ForEach(func(name []byte, bSrc *bbolt.Bucket) error {
				extr := false
				for _, b := range buckets {
					if b.Match(name) {
						extr = true
						break
					}
				}
				if extr {
					log.Info("Extracting bucket:", string(name))
					bDst, err := txDest.CreateBucket(name)
					if err != nil {
						return xerrors.Errorf("couldn't create bucket: %v", err)
					}
					return bSrc.ForEach(func(k, v []byte) error {
						return bDst.Put(k, v)
					})
				}
				return nil
			})
		})
	})
	if err != nil {
		return xerrors.Errorf("error while copying: %v", err)
	}
	if err := dbSrc.Close(); err != nil {
		return xerrors.Errorf("couldn't close source DB: %v", err)
	}
	if err := dbDst.Close(); err != nil {
		return xerrors.Errorf("couldn't close destination DB: %v", err)
	}
	return nil
}
