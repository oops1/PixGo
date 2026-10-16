package i18n

var accessEn = map[string]string{
	"access.favorite":    "Favorite",
	"access.slideshow":   "Slideshow progress",
	"access.refresh":     "Refresh",
	"access.go":          "Go",
	"access.places":      "Places",
	"access.items":       "Images and folders",
	"access.statusBar":   "Status bar",
	"access.thumbnail":   "Thumbnail",
	"access.menu":        "Menu",
	"access.loadingFull": "Loading full resolution",
	"cmd.panLeft":        "Pan left",
	"cmd.panRight":       "Pan right",
	"cmd.panUp":          "Pan up",
	"cmd.panDown":        "Pan down",
}

var accessRu = map[string]string{
	"access.favorite":    "Избранное",
	"access.slideshow":   "Ход слайд-шоу",
	"access.refresh":     "Обновить",
	"access.go":          "Перейти",
	"access.places":      "Места",
	"access.items":       "Изображения и папки",
	"access.statusBar":   "Строка состояния",
	"access.thumbnail":   "Миниатюра",
	"access.menu":        "Меню",
	"access.loadingFull": "Загрузка полного разрешения",
	"cmd.panLeft":        "Сдвинуть влево",
	"cmd.panRight":       "Сдвинуть вправо",
	"cmd.panUp":          "Сдвинуть вверх",
	"cmd.panDown":        "Сдвинуть вниз",
}

func init() {
	for k, v := range accessEn {
		en[k] = v
	}
	for k, v := range accessRu {
		ru[k] = v
	}
}
