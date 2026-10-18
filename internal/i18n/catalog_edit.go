package i18n

var editEn = map[string]string{
	"cmd.edit":   "Edit",
	"cmd.crop":   "Crop…",
	"cmd.resize": "Resize…",

	"edit.bar":          "Editing tools",
	"edit.aspect":       "Aspect ratio",
	"edit.aspect.free":  "Free",
	"edit.aspect.orig":  "Original",
	"edit.aspect.1x1":   "Square",
	"edit.aspect.4x3":   "4:3",
	"edit.aspect.3x2":   "3:2",
	"edit.aspect.16x9":  "16:9",
	"edit.swap":         "Swap portrait and landscape",
	"edit.resize":       "Resize…",
	"edit.reset":        "Reset",
	"edit.saveCopy":     "Save a copy",
	"edit.save":         "Save",
	"edit.saveAs":       "Save as…",
	"edit.saveMenu":     "More ways to save",
	"edit.cancel":       "Cancel",
	"edit.cropArea":     "Crop area",
	"edit.cropValue":    "%d, %d, %d × %d",
	"edit.size":         "%d × %d",
	"edit.sizeResized":  "%d × %d (resized)",
	"edit.snapped":      "Snapped to the JPEG blocks",
	"edit.handle.hint":  "Arrows move the area, Ctrl+arrows resize it",
	"edit.discard":      "Discard the changes?",
	"edit.discardText":  "The changes you made will not be saved.",
	"edit.discardYes":   "Discard",
	"edit.busy":         "Saving…",
	"edit.noEncoder":    "This format cannot be written. The copy will be saved as PNG.",
	"edit.overwriteOff": "This format cannot be written, so the original cannot be replaced",

	"edit.refused.animated": "Animated pictures cannot be edited",
	"edit.refused.vector":   "Vector pictures (SVG) cannot be edited",
	"edit.refused.loading":  "The picture is still loading",
	"edit.refused.none":     "Open a picture first",
	"edit.refused.partial":  "This picture is shown in parts and is too large to edit",

	"error.editTooLarge": "The chosen area is too large to process. Choose a smaller area or reduce the size.",
	"error.editRead":     "Cannot read the picture for editing",
	"error.editSource":   "The file changed after it was shown. Open it again.",

	"dialog.overwrite.title":   "Replace the original?",
	"dialog.overwrite.message": "The file \"%s\" will be replaced with the edited picture. This cannot be undone.",
	"dialog.overwrite.yes":     "Replace",
	"dialog.dontAskAgain":      "Don't ask again",

	"toast.copySaved":       "Copy saved: %s",
	"toast.overwritten":     "Changes saved",
	"toast.savedNoExif":     "Saved without camera data: it did not fit into the file",
	"toast.copySavedNoExif": "Copy saved: %s. The camera data did not fit into the file",

	"save.stripGPS": "Remove location data (GPS)",

	"resize.title":    "Resize",
	"resize.width":    "Width",
	"resize.height":   "Height",
	"resize.unit":     "Units",
	"resize.unit.px":  "Pixels",
	"resize.unit.pct": "Percent",
	"resize.keep":     "Keep proportions",
	"resize.original": "Original: %d × %d",
	"resize.result":   "Result: %d × %d",
	"resize.range":    "From 1 to %d pixels on a side, up to %d megapixels",
	"resize.apply":    "Apply",
	"resize.reset":    "Original size",
	"resize.invalid":  "Enter a number",
	"resize.tooBig":   "Too large",
}

var editRu = map[string]string{
	"cmd.edit":   "Изменить",
	"cmd.crop":   "Обрезать…",
	"cmd.resize": "Изменить размер…",

	"edit.bar":          "Инструменты изменения",
	"edit.aspect":       "Пропорции",
	"edit.aspect.free":  "Свободные",
	"edit.aspect.orig":  "Исходные",
	"edit.aspect.1x1":   "Квадрат",
	"edit.aspect.4x3":   "4:3",
	"edit.aspect.3x2":   "3:2",
	"edit.aspect.16x9":  "16:9",
	"edit.swap":         "Поменять книжную и альбомную ориентацию",
	"edit.resize":       "Изменить размер…",
	"edit.reset":        "Сбросить",
	"edit.saveCopy":     "Сохранить копию",
	"edit.save":         "Сохранить",
	"edit.saveAs":       "Сохранить как…",
	"edit.saveMenu":     "Другие способы сохранения",
	"edit.cancel":       "Отмена",
	"edit.cropArea":     "Область обрезки",
	"edit.cropValue":    "%d, %d, %d × %d",
	"edit.size":         "%d × %d",
	"edit.sizeResized":  "%d × %d (с новым размером)",
	"edit.snapped":      "Выровнено по блокам JPEG",
	"edit.handle.hint":  "Стрелки двигают область, Ctrl+стрелки меняют её размер",
	"edit.discard":      "Отменить изменения?",
	"edit.discardText":  "Внесённые изменения не будут сохранены.",
	"edit.discardYes":   "Отменить",
	"edit.busy":         "Сохранение…",
	"edit.noEncoder":    "Этот формат нельзя записать. Копия будет сохранена в PNG.",
	"edit.overwriteOff": "Этот формат нельзя записать, поэтому заменить оригинал нельзя",

	"edit.refused.animated": "Анимированные изображения нельзя изменять",
	"edit.refused.vector":   "Векторные изображения (SVG) нельзя изменять",
	"edit.refused.loading":  "Изображение ещё загружается",
	"edit.refused.none":     "Сначала откройте изображение",
	"edit.refused.partial":  "Изображение показано по частям и слишком велико для изменения",

	"error.editTooLarge": "Выбранная область слишком велика для обработки. Выберите меньшую область или уменьшите размер.",
	"error.editRead":     "Не удалось прочитать изображение для изменения",
	"error.editSource":   "Файл изменился после показа. Откройте его заново.",

	"dialog.overwrite.title":   "Заменить оригинал?",
	"dialog.overwrite.message": "Файл «%s» будет заменён изменённым изображением. Отменить это нельзя.",
	"dialog.overwrite.yes":     "Заменить",
	"dialog.dontAskAgain":      "Больше не спрашивать",

	"toast.copySaved":       "Копия сохранена: %s",
	"toast.overwritten":     "Изменения сохранены",
	"toast.savedNoExif":     "Сохранено без данных камеры: они не поместились в файл",
	"toast.copySavedNoExif": "Копия сохранена: %s. Данные камеры не поместились в файл",

	"save.stripGPS": "Удалить сведения о месте (GPS)",

	"resize.title":    "Изменить размер",
	"resize.width":    "Ширина",
	"resize.height":   "Высота",
	"resize.unit":     "Единицы",
	"resize.unit.px":  "Пиксели",
	"resize.unit.pct": "Проценты",
	"resize.keep":     "Сохранять пропорции",
	"resize.original": "Исходный: %d × %d",
	"resize.result":   "Результат: %d × %d",
	"resize.range":    "От 1 до %d пикселей по стороне, не более %d мегапикселей",
	"resize.apply":    "Применить",
	"resize.reset":    "Исходный размер",
	"resize.invalid":  "Введите число",
	"resize.tooBig":   "Слишком велико",
}

func init() {
	for k, v := range editEn {
		en[k] = v
	}
	for k, v := range editRu {
		ru[k] = v
	}
}
