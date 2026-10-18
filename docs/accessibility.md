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
| Ctrl+E | Edit: crop, resize, rotate and flip (see below) |
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

### Edit mode

Ctrl+E (the pencil in the toolbar, **Crop** and **Resize** in the menu) switches the viewer to the edit mode. The toolbar and the arrows are replaced by the edit bar; the viewer commands (zoom, previous and next, delete, gallery, full screen) are off until you leave.

| Key | Action |
|---|---|
| Arrows, Shift+arrows | Move the crop area by 1 pixel, by 10 pixels |
| Ctrl+Left, Right, Up, Down | Resize the area from its bottom-right corner by 1 pixel (Ctrl+Shift: 10); with a fixed aspect ratio the other side follows |
| Tab, Shift+Tab | Walk the crop area and the controls of the edit bar: aspect ratio, swap portrait and landscape, rotate left, rotate right, flip horizontally, flip vertically, **Resize**, **Reset**, **Save a copy**, its arrow, **Cancel**; disabled controls are skipped |
| Enter, Space | On a control of the bar: press it. On the crop area, Enter saves a copy |
| R, Shift+R, H, V | Rotate right, rotate left, flip horizontally, flip vertically (the crop area follows) |
| Ctrl+Shift+S | Save as |
| Esc | Cancel; asks first when something was changed |

The arrow of **Save a copy** opens a menu with **Save** (replaces the original after a confirmation) and **Save as**. The aspect ratio list (Free, Original, Square, 4:3, 3:2, 16:9) and the arrow menu are ordinary menus: Up, Down, Enter, Esc.

The **Resize** dialog has two fields (width, height), the units (pixels or percent) and **Keep proportions**. Tab goes through the width, the height, the two units, the check box, **Apply** and **Cancel**; Up and Down in a field change the number by 1 (Shift: 10); Enter applies, Esc cancels. The confirmation of replacing the original has **Replace**, **Cancel** and **Don't ask again**.

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

Tab and Shift+Tab move between the buttons, Enter presses the default button, Esc cancels. The **Save options** dialog has the quality slider (arrows, Home, End, PgUp, PgDn), the **Remove location data (GPS)** check box and, for formats that offer it, the **Lossless** check box.

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
| Edit mode: crop area | group | named "Crop area"; the value is "x, y, width × height" in pixels of the picture; its description names the keys |
| Edit mode: edit bar | buttons, combo box | the aspect ratio is a combo box whose value is the chosen ratio; every button is named; Save a copy and its arrow, Reset and the buttons that have nothing to do are disabled |
| Edit mode: size of the result | label | "width × height" of the file that will be written |
| Resize dialog | group | width and height are text fields with their value, the units are radio buttons, **Keep proportions** is a check box; the original and the resulting size are labels |
| Popup menu | menu | items are menu items (check marks and sub-menus are states); the shortcut is the description; the highlighted item is focused |

### Actions

A screen reader can act on the individual controls of every page that PixGo draws itself, not only read them. Each interactive node has a stable identifier and the page carries out the request exactly as it would carry out a click or a key:

| Request | Meaning |
|---|---|
| Invoke (press) | The same as a click: the buttons of the title bar, toolbar, arrows and slideshow bar run their command; in the gallery a place goes to the folder, an item is opened (like a double click), the address bar starts editing, the filter and size buttons open their menu; in the information panel the copy buttons and links work; in the settings buttons, links and drop-down lists work; in the save options dialog Save and Cancel work; in the edit mode the buttons of the bar work, the arrow and the aspect ratio open their menu, and the crop area takes Focus; the Resize dialog and the confirmation of replacing the original are driven the same way |
| Toggle | Switches a check box: information, favorite, the check boxes of the settings and the formats, **Lossless**, **Remove location data (GPS)**, **Keep proportions**, **Don't ask again** |
| Select | Chooses a radio button or a list row: list and grid view, the places and items of the gallery (selects, does not open), the sections and options of the settings |
| Focus | Moves the keyboard focus to the control and shows the focus ring, scrolling it into view when needed; in the gallery it selects the item, in the settings it switches to the section |
| Expand | Opens the list of a drop-down in the settings and the sub-menu of a menu item |

A disabled control, an identifier that no longer exists and a gallery row that has scrolled out of the list refuse the request. The window buttons (minimize, maximize, close) can be pressed but are not tab stops, so they refuse Focus. A slider takes Focus only; change it with the arrow keys.

Known limits:

- The window of a dialog that the engine shows (confirmation, errors, file dialogs) has no name and its title is not published; its buttons and text are.
- UI Automation does not receive the selected state of radio buttons from the engine, so on Windows a selected option is not announced as selected.
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

- The semantics are in the files `access.go` of the packages `ui/window`, `ui/gallery`, `ui/infopanel`, `ui/settings`, `ui/saveopts`, `ui/view`, `ui/edit`, `ui/resize` and `ui/confirm`, and in `app/access_menu.go` (the menu is the engine's `PopupMenu`, which publishes its own items). They implement the engine interfaces `widget.Accessible` and `widget.AccessChildrenProvider`; the actions are in `access_actions.go` next to them (`widget.AccessActionHandler`, identifiers in `AccessInfo.ID`). The tests build the tree with the engine, call `Engine.AccessAction` on the nodes and compare the effect with a real click (`testutil.AccessDo`, `testutil.RequireActionIDs`). They also check that every interactive node has a name, that no name is a bare translation key and that the names follow the language.
- The focus ring is `gfx.Ring`; the colors come from the theme (`Focus`, and `Canvas` for the inner line of the high-contrast theme).
- `PIXGO_SNAPSHOTS=<dir> go test ./internal/app -run TestEveryStateRendersAtEveryScale` writes a picture of every page, dialog, menu and the focus ring at 1.0, 1.25, 1.5 and 2.0 in both themes. `PIXGO_SNAPSHOT_LANG=ru` selects the language.
