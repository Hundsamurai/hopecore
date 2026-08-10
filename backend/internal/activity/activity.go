// Package activity отвечает за активность вакансии: как её вычислять
// и как определять по HTTP-ответу сайта.
//
// HTTP-проверка добавляется в Task 5; здесь пока правило вычисления
// эффективного значения из двух nullable-полей.
package activity

// Resolve вычисляет отображаемую активность вакансии и признак расхождения.
//
// Правило (docs/main/design-stage1.md, п. 6):
//
//	effective = manual ?? auto ?? true
//
// Новая вакансия считается активной, пока не доказано обратное: пользователь
// добавил её вручную, значит она интересна.
//
// conflict = true, когда ручное решение расходится с результатом последней
// авто-проверки. Это не повод что-то исправлять автоматически — UI показывает
// подсказку «сайт говорит другое», а решение остаётся за пользователем.
func Resolve(auto, manual *bool) (effective bool, conflict bool) {
	switch {
	case manual != nil:
		effective = *manual
	case auto != nil:
		effective = *auto
	default:
		effective = true
	}

	conflict = manual != nil && auto != nil && *manual != *auto
	return effective, conflict
}
