-- ClaudeWrap integration for Neovim
-- Requires: toggleterm.nvim (https://github.com/akinsho/toggleterm.nvim)
--
-- Add to your lazy.nvim or packer setup, or paste into your init.lua after
-- toggleterm is loaded.
--
-- When claudewrap detects $NVIM it runs in passthrough mode (transparent
-- stdio, no TUI) so the terminal buffer behaves like a normal shell.

local Terminal = require("toggleterm.terminal").Terminal

local claudewrap = Terminal:new({
  cmd = "claudewrap",
  direction = "float",
  float_opts = {
    border = "curved",
    width = math.floor(vim.o.columns * 0.9),
    height = math.floor(vim.o.lines * 0.9),
  },
  on_open = function()
    vim.cmd("startinsert!")
  end,
})

vim.keymap.set("n", "<leader>cc", function()
  claudewrap:toggle()
end, { desc = "Toggle ClaudeWrap", noremap = true, silent = true })

-- If you use vim-floaterm instead of toggleterm, add this to your config:
--
--   vim.keymap.set("n", "<leader>cc", ":FloatermNew --title=ClaudeWrap --width=0.9 --height=0.9 claudewrap<CR>",
--     { noremap = true, silent = true })
