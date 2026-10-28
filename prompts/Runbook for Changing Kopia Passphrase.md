# Runbook for Changing Kopia Passphrase

*Created: 2026-09-23, 10:58:34 AM*

---

**You**

write me runbook for changing kopia passphrase on repo whose age precludes the kopia changepassword command. 

---

**Assistant**

 Here is a runbook for changing the passphrase on a Kopia repository that is too old to use the native `kopia repository change-password` command.

> **Important context:** Kopia uses envelope encryption. Your passphrase does not encrypt the backup data directly; it encrypts the repository’s master key, which lives in the `kopia.repository` and `kopia.blobcfg` format blobs [^1][^2]. The `change-password` command simply re-encrypts those format blobs with a key derived from the new passphrase [^3][^4]. If your repository predates that feature, the safest paths are to either upgrade the repository format first or migrate to a fresh repository.

---

## Runbook: Changing Passphrase on an Old Kopia Repository

### 1. Pre-flight checks

| Step | Command / Action | Purpose |
|------|------------------|---------|
| Verify current access | `kopia repo status` | Confirm you can open the repo with the current passphrase |
| Note format version | `kopia repo status \| grep -i "format"` | Determine if the repo can be upgraded |
| Record connection details | `kopia repo status -t -s` | Save the full reconnect command (minus secrets) [^5] |
| Pause maintenance | `kopia maintenance set --owner ""` (or pause cron jobs) | Prevent concurrent writes during the operation |
| Backup local config | Copy `~/.config/kopia/repository.config` and `.kopia-cache` | Allow quick rollback if reconnect fails |

### 2. Determine why `change-password` is unavailable

Run the native command to capture the exact error:

```bash
kopia repository change-password --new-password "$NEW_PW"
```

Common outcomes for old repositories:

- **Format version too old** → proceed to Option A (upgrade then change password).
- **Command not present in your Kopia binary** → upgrade your Kopia CLI to a current release and retry.
- **Repository uses a legacy master-key layout** → use Option B (migrate to a new repo), which is the safest route.

You can check supported format versions with:

```bash
kopia repository create filesystem --help | grep -i "format-version"
```

Kopia currently supports format versions `1`, `2`, and `3` [^6].

---

### Option A: Upgrade repository format, then run `change-password`

This is the preferred option if the repository can be upgraded without rewriting pack blobs.

1. **Connect to the repository with the current passphrase:**

   ```bash
   export KOPIA_PASSWORD="$OLD_PW"
   kopia repo connect <provider> ... # or use existing config
   ```

2. **Set an upgrade lock and upgrade the format:**

   ```bash
   kopia repository upgrade begin
   kopia repository upgrade advance
   kopia repository upgrade finish
   ```

   > Use the upgrade commands appropriate to your Kopia version; the CLI names have evolved around the format-version-3 work [^7].

3. **Verify the new format version:**

   ```bash
   kopia repo status
   ```

4. **Change the passphrase using the native command:**

   ```bash
   kopia repository change-password --new-password "$NEW_PW"
   ```

   The command will rewrite `kopia.repository` and `kopia.blobcfg`. **Do not interrupt it** — a partial write can leave the repo inaccessible with either password [^4].

5. **Disconnect and reconnect with the new passphrase:**

   ```bash
   kopia repo disconnect
   export KOPIA_PASSWORD="$NEW_PW"
   kopia repo connect <provider> ...
   kopia repo status
   ```

---

### Option B: Create a new repository and migrate snapshots (safest fallback)

If the repository cannot be upgraded, create a new repository with the desired passphrase and migrate the snapshot history.

1. **Connect to the old repository:**

   ```bash
   export KOPIA_PASSWORD="$OLD_PW"
   kopia repo connect <provider> --config-file ~/.config/kopia/repo-old.config ...
   ```

2. **Create a new repository with the new passphrase:**

   ```bash
   export KOPIA_PASSWORD="$NEW_PW"
   kopia repo create <provider> --config-file ~/.config/kopia/repo-new.config ...
   ```

3. **Migrate snapshots from old to new:**

   ```bash
   kopia snapshot migrate --source-config ~/.config/kopia/repo-old.config \
                          --destination-config ~/.config/kopia/repo-new.config \
                          --all
   ```

4. **Verify migration:**

   ```bash
   export KOPIA_PASSWORD="$NEW_PW"
   kopia repo connect --config-file ~/.config/kopia/repo-new.config ...
   kopia snapshot list
   kopia content verify
   ```

5. **Repoint backup jobs** to the new repository config and new passphrase.

6. **Keep the old repository** (read-only) until you have confirmed restore paths from the new one.

---

### Option C: Manual re-encryption of format blobs (expert only)

> **Warning:** This is not officially supported and can permanently lock you out. Only use this if you cannot upgrade and cannot afford the storage/time of a full migration.

Because the passphrase only protects the master key in `kopia.repository` and `kopia.blobcfg` [^1][^2], the theoretical manual procedure is:

1. Download the format blobs:
   - `kopia.repository`
   - `kopia.blobcfg`

2. Decrypt them with the old passphrase-derived key.

3. Re-encrypt them with a key derived from the new passphrase.

4. Upload them back to the storage backend, ensuring consistency and correct blob IDs.

**Do not attempt this on a production repository without a complete, tested backup.** A single mistake in salt, key-derivation parameters, or blob metadata will render the repository unreadable.

---

## Post-change validation

| Check | Command |
|-------|---------|
| Reconnect with new passphrase | `kopia repo connect ...` |
| List snapshots | `kopia snapshot list` |
| Verify content integrity | `kopia content verify` |
| Test restore a small file | `kopia restore ...` |
| Update password manager / automation | Store `$NEW_PW` and any `KOPIA_PASSWORD` references |
| Resume maintenance | `kopia maintenance set --owner <host>` |

## Rollback notes

- If `change-password` fails partway through, **both passwords may be invalid** [^4]. Restore the original `kopia.repository` and `kopia.blobcfg` blobs from storage-level backups if you have them.
- If you chose Option B, the old repository remains untouched and can be reconnected with the old passphrase.

[^1]: https://kopia.io/docs/advanced/encryption/
[^2]: https://github.com/kopia/kopia/issues/309
[^3]: https://kopia.io/docs/reference/command-line/common/repository-change-password/
[^4]: https://github.com/kopia/kopia/issues/3049
[^5]: https://kopia.io/docs/reference/command-line/
[^6]: https://kopia.io/docs/reference/command-line/common/repository-create-filesystem/
[^7]: https://github.com/kopia/kopia/pull/1818

**References**

[^1]: [Encryption - Kopia](https://kopia.io/docs/advanced/encryption/) (17%)
[^2]: [[Feature] Add support for changing the passphrase of the repository ...](https://github.com/kopia/kopia/issues/309) (10%)
[^3]: [repository change-password - Kopia](https://kopia.io/docs/reference/command-line/common/repository-change-password/) (19%)
[^4]: [Change-password failure can lock a user out of their ...](https://github.com/kopia/kopia/issues/3049) (6%)
[^5]: [Command-Line Reference - Kopia](https://kopia.io/docs/reference/command-line/) (23%)
[^6]: [repository create filesystem](https://kopia.io/docs/reference/command-line/common/repository-create-filesystem/) (7%)
[^7]: [feat(cli): New Upgrade CLI / Switch to Format Version 3 (upgrade...](https://github.com/kopia/kopia/pull/1818) (18%)

---

