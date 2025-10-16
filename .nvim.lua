-- Project-local DAP configuration for pi-player
-- This file is automatically loaded by LazyVim when you open this project
--
-- Debug configurations are defined in .vscode/launch.json for cross-editor compatibility
-- See DEBUG.md for usage instructions

-- Usage:
-- 1. SSH to target machine with port forwarding:
--    make debug-tunnel HOST=user@target-machine
-- 2. On target machine, start delve:
--    systemctl --user stop pi-player
--    dlv exec ~/.local/bin/pi-player --headless --listen=:2345 --api-version=2 --accept-multiclient
-- 3. In nvim, start debugging with :DapContinue or <leader>dc

-- Load VSCode launch.json configurations
-- This allows the same debug config to work in both VSCode and Neovim
require('dap.ext.vscode').load_launchjs(nil, { go = {'go'} })

print("pi-player DAP config loaded from .vscode/launch.json")
