# nodestat

Performs eth-like nodes checks

## Usage

```bash
nodestat <eth|bsc|poly|arb|all>
```

First argument is node name or show stats for all nodes.


## Building and adding to PATH (fish)

1. **Compile your Go script into a binary**:

   Compile your script into a binary by running:

    ```bash
    go build .
    ```

2. **Move the binary to a directory in your PATH**:

   Create a directory for your custom binaries. You can create a `bin` directory in your home directory:

    ```bash
    mkdir ~/bin
    ```

   Move the `nodestat` binary to this directory:

    ```bash
    mv nodestat ~/bin
    ```

3. **Add the directory to your PATH in Fish shell**:

   Open your Fish shell configuration file `~/.config/fish/config.fish`:

    ```bash
    nano ~/.config/fish/config.fish
    ```

   Add the following line to this file to include the `bin` directory in your PATH:

    ```fish
    set -gx PATH $HOME/bin $PATH
    ```

   Save the changes to the configuration file.

4. **Reload Fish configuration**:

   After saving the changes, reload your Fish configuration to apply the changes:

    ```bash
    source ~/.config/fish/config.fish
    ```

5. **Test your setup**:

   You should now be able to run `gacli` from anywhere in your terminal, and it should execute your `gitfeat` script with the specified task name.
