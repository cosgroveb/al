# Your first list

Create a practice list, check and uncheck one item, then remove the practice
list from your account. Start with [al installed](../how-to/install.md) and
your AnyList account credentials.

## Check your connection

Open a terminal and start a Bash session for this tutorial:

```sh
bash
```

Enter your email address at the prompt:

```bash
read -r -p 'AnyList email: ' ANYLIST_EMAIL
```

Enter your password at this hidden prompt. Neither prompt response enters
your shell history.

```bash
read -r -s -p 'AnyList password: ' ANYLIST_PASSWORD
```

Export the credentials and check your connection:

```bash
printf '\n'
export ANYLIST_EMAIL ANYLIST_PASSWORD
al auth status
```

A successful check prints:

```text
Authenticated with AnyList.
```

Keep using this terminal for the remaining steps.

## Create a practice list

Create a list with a name specific to this session:

```bash
al_practice_name="al practice $(date -u +%Y%m%dT%H%M%SZ)-$$-$RANDOM"
al list create "$al_practice_name"
```

The result starts with `created`, followed by the practice list's name and
its ID in parentheses. This creates an unshared list.

Copy that list ID without the parentheses. Run this command and paste the
ID at the prompt:

```bash
read -r -p 'Practice list ID: ' al_practice_id
```

View the new list:

```bash
al items --list-id "$al_practice_id"
```

Confirm that the heading contains the practice list's name. Under it, al
prints `No items.` Keep this list ID for the remaining commands.

## Add an item

Add milk and view the list:

```bash
al add --list-id "$al_practice_id" milk
al items --list-id "$al_practice_id"
```

The list now contains `[ ] milk` followed by its item ID. The empty checkbox
means the item is unchecked.

## Check and uncheck the item

Check milk and view the remaining unchecked items:

```bash
al check --list-id "$al_practice_id" milk
al items --list-id "$al_practice_id"
```

The list now shows `No items.` View checked items too:

```bash
al items --list-id "$al_practice_id" --all
```

Milk still exists in the list, with `[x]` beside its name. Uncheck it:

```bash
al uncheck --list-id "$al_practice_id" milk
al items --list-id "$al_practice_id"
```

The unchecked view contains `[ ] milk` again.

## Remove the practice list

Confirm that the last output's heading names your practice list. Remove
that list using the ID you saved:

```bash
al list delete --id "$al_practice_id"
al lists
```

The deletion result starts with `removed`. The practice list no longer
appears in `al lists`. The commands above target only the practice list and
leave your household lists in place.

Leave the tutorial session to discard the credentials you entered:

```bash
exit
```

Use the [command reference](../reference/README.md) to look up commands and
flags. Read [list and item behavior](../explanation/behavior.md) for how al
selects targets and handles changes.
