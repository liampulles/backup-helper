# Backup Helper

This is a fairly simple wrapper around [cshatag](https://github.com/rfjakob/cshatag) and [rsync](https://rsync.samba.org/).

My usecase is that I have a single drive which I would like to backup to another drive. Noth drives are mounted to seperate folders, and I would use the backup-helper like so: `backup-helper /mnt/source /mnt/backup`.

This will:
* Check both folders contain a `.backup-helper-check` file (smoke test to ensure that both drives are both mounted)
* Check that both folders allow for writing and reading
* Run `cshatag` on both drives (in parallel) to check for bitrot
* Run `rsync` to ensure that the contents of `/mnt/backup` are updated to contain exactly what is in `/mnt/source` (but not the other way around)

The program will stop if any step above fails. In all cases, the program will send an email report, as configured in `config.json` (see `config.json.example`)