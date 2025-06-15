# Backup Helper

This is a fairly simple wrapper around [rsync](https://rsync.samba.org/).

My usecase is that I have a mounted drive which I would like to backup to another mounted drive. I might use the backup-helper like so:

```shell
backup-helper /mnt/source /mnt/backup
```

This will:
1. Check both folders contain a `.backup-helper-check` file (smoke test to ensure that both drives are both mounted)
1. Check that both folders allow for writing and reading
1. Run `rsync` to sync the contents of `/mnt/source` to `/mnt/backup` (but not the other way around)

The program will stop if any step above fails. In all cases, the program will send an email report, as configured in `config.json` (see `config.json.example`)