# Accessibility

PixGo can be used without a mouse, with a screen reader, in the high-contrast theme and on displays with any scale from 100 % to 200 %. This page lists what is supported, how to drive the program from the keyboard and what is known not to work yet.

## Keyboard

Every function that the mouse reaches is reachable from the keyboard. Disabled controls are skipped by Tab. Enter and Space press the control that has the focus. A visible focus ring is drawn only while you use the keyboard; a mouse click hides it.

### Viewer

| Key | Action |
|---|---|
| Tab, Shift+Tab | Move the focus through the controls (order below) |
| Enter, Space | Press the focused control |
| Esc | Step back: focused control, then the image; closes the information panel; leaves full screen and stops the slideshow |
| Left, PgUp / Right, PgDn | Previous / next image |
| Home, End | First / last image |
| `+`, `-`, `0`, `1`, `2` | Zoom in, zoom out, actual size, fit to window, fit to width |
| Ctrl+Left, Right, Up, Down | Pan a zoomed image by a tenth of the window |
| R, Shift+R | Rotate right / left |
| H, V | Flip horizontally / vertically |
| Ctrl+I | Information panel |
| Ctrl+D | Add to / remove from favorites |
| Ctrl+G, Ctrl+O | Gallery |
| Ctrl+, | Settings |
| Ctrl+Shift+S | Save as |
| Ctrl+C, Ctrl+Shift+C, Ctrl+Shift+R | Copy the file, copy the path, show in the folder |
| Del, Shift+Del | Move to the recycle bin, delete permanently |
| F11, Alt+Enter | Full screen |
| F5, Space | Start the slideshow; pause and resume it |
| Shift+F10, Menu key | Open the menu (the same as the **More** button) |

Rotate 180°, Fit to window, Fit to width and 100 % are also in the menu (the **More** button, Shift+F10 or the Menu key), next to the other rotate and zoom commands.

The focus visits the controls in this order and wraps around: the buttons of the title bar (back, information, favorite, more), the image, the toolbar (gallery, delete, rotate left, zoom out, zoom, zoom in, rotate right, more, full screen), the arrows to the previous and next image, and the controls of the information panel when it is open. In full screen the order is the image, play or pause, slideshow settings, stop the slideshow, the two arrows and the exit button. The window buttons (minimize, maximize, close) are not tab stops; use the keys of the system (Alt+Space opens the window menu on Windows, Alt+F4 closes the window).

When the information panel is opened from the keyboard with Enter, the focus moves into it: Tab goes through the close button, the copy buttons and the links, Up and Down scroll, Esc closes it and returns to the information button.

The arrows keep their meaning (previous and next image) even when a control has the focus.

### Menus

While a menu is open, Up and Down move through the items, Enter runs the item and Esc closes the menu. Tab is ignored. The focus returns to the image when the menu closes.

### Gallery

| Key | Action |
|---|---|
| Tab, Shift+Tab | Back, forward, up, address bar, filter, refresh, list and grid view, thumbnail size, places, items, **Open**, the arrow of **Open** |
| Enter, Space | Press the focused button; on the address bar start editing |
| Arrows, Home, End, PgUp, PgDn | Move the selection in the items |
| Enter | Open the selected item |
| Left, Right | Switch between the list and the grid view when the view switch has the focus |
| Up, Down, Home, End, Enter | Move in the places and go to the place |
| Ctrl+L, F4 | Edit the address; Enter goes there, Esc cancels |
| Alt+Left, Alt+Right, Alt+Up, Backspace | Back, forward, up |
| F5, Ctrl+R | Refresh |
| Ctrl+D | Favorite |
| Del | Delete the selected file |
| Menu key | Menu of the selected item |
| Letters | Jump to the item whose name starts with them (when the items have the focus) |
| Esc | Return to the viewer |

### Settings

| Key | Action |
|---|---|
| Tab, Shift+Tab | Back button, section list, then the controls of the section |
| Ctrl+Tab, Ctrl+Shift+Tab, Ctrl+PgDn, Ctrl+PgUp | Next / previous section |
| Arrows, Home, End | In the section list: switch the section |
| Up, Down | On a button, a check box or a switch: the previous / next control |
| Arrows | In a group of options (theme, background): move and select |
| Space, Enter | Switch a check box, press a button, open a drop-down list |
| Left, Right, Up, Down, Home, End, PgUp, PgDn | Change a slider |
| Up, Down | On a drop-down list: change the value without opening it |
| Alt+Down | Open the drop-down list |
| Esc, Backspace, Alt+Left | Back |

### Dialogs

Tab and Shift+Tab move between the buttons, Enter presses the default button, Esc cancels. The **Save options** dialog has the quality slider (arrows, Home, End, PgUp, PgDn) and, for formats that offer it, the **Lossless** check box.

## Screen readers

PixGo publishes its controls through UI Automation on Windows and AT-SPI on Linux. Every page gives its elements a role, a name in the language of the interface (Russian or English), a state (focused, selected, checked, disabled) and a value where it has one. Names are the texts of the tooltips and the shortcuts are their descriptions; the favorite toggle is named Favorite and reports its state instead of changing its name.

| Element | Role | Notes |
|---|---|---|
| Window | window | named PixGo |
| Title bar, toolbar, arrows, slideshow bar, window buttons | button | the shortcut is the description; zoom shows its percentage as the value |
| Information, favorite | check box | checked when the panel is open / the file is a favorite |
| File name and size in the title bar | label | |
| Picture | image | named like the title ("name — width × height"), the value is the zoom; a preview is described as such |
| Empty, loading and error states, the preview badge, notifications | label | |
| Slideshow counter and progress | label, progress bar | progress is a number from 0 to 1 |
| Gallery: address bar | edit | read-only until edited |
| Gallery: filter, thumbnail size | button | the value is the chosen format / size |
| Gallery: list and grid view | radio buttons | |
| Gallery: places and items | lists of radio buttons, headers are labels | selected state; only the visible items are published; a folder says so |
| Gallery: status bar | label | |
| Information panel | group | rows are labels (name and value); the location and the map link are buttons; every copy button is named |
| Settings | group | sections are radio buttons, options are check boxes, radio buttons, combo boxes and sliders (the value is the shown text) |
| Popup menu | group | items are buttons, check boxes or radio buttons; the shortcut is the description |

Known limits:

- A screen reader can read every element but cannot press or focus an individual control on a page that PixGo draws itself (the viewer chrome, the gallery, the settings, the information panel). The engine sends such a request to the page as a whole. Use the keyboard.
- The window of a dialog that the engine shows (confirmation, errors, file dialogs) has no name and its title is not published; its buttons and text are.
- UI Automation does not receive the selected state of radio buttons from the engine, so on Windows a selected option is not announced as selected.
- The item of a menu that is highlighted with the arrows is not announced; the menu is announced as a whole.
- Items of a menu that does not fit the window are listed completely, but the positions of the hidden ones are only estimates.

## High contrast

With the high-contrast theme the focus ring is yellow, 2 px wide, with a 1 px black line between it and the control, and every color of the interface comes from the high-contrast palette. Text size follows the system: the **Text size** setting of Windows and the font of the desktop on Linux.

## Display scale

PixGo follows the scale of the display: 100 %, 125 %, 150 %, 175 %, 200 % and the values in between. Text, icons, borders, rounded corners and thumbnails are drawn at the real pixel size of the display, so a line stays one pixel wide where the scale allows and nothing is stretched.

- On Windows the program is per-monitor DPI aware (version 2). Dragging the window to a monitor with another scale redraws it at once at the new scale, and the size of the window that is saved on exit does not change from one scale to another.
- The size of the text is the system message font (9 pt) multiplied by the **Text size** setting. It is a size in points and is not multiplied by the display scale a second time.
- `pixgo -scale 1.5` starts the program with a fixed scale; the accepted values are 1, 1.25, 1.5, 1.75 and 2. With `-screenshot` it draws one picture at that scale (the picture is as many times larger as the scale):

```
pixgo -screenshot out.png -width 1000 -height 640 -theme dark -lang en -scale 1.5 photo.jpg
```

## For developers

- The semantics are in the files `access.go` of the packages `ui/window`, `ui/gallery`, `ui/infopanel`, `ui/settings`, `ui/view` and `app` (the menu). They implement the engine interfaces `widget.Accessible` and `widget.AccessChildrenProvider`. The tests build the tree with the engine and check that every interactive node has a name, that no name is a bare translation key and that the names follow the language.
- The focus ring is `gfx.Ring`; the colors come from the theme (`Focus`, and `Canvas` for the inner line of the high-contrast theme).
- `PIXGO_SNAPSHOTS=<dir> go test ./internal/app -run TestEveryStateRendersAtEveryScale` writes a picture of every page, dialog, menu and the focus ring at 1.0, 1.25, 1.5 and 2.0 in both themes. `PIXGO_SNAPSHOT_LANG=ru` selects the language.
