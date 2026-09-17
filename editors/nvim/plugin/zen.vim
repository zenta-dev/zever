" Guard file for plugin managers that expect a plugin/ entrypoint.
if exists('g:loaded_zever')
  finish
endif
let g:loaded_zever = 1
