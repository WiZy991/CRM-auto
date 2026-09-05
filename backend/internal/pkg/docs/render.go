package docs

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"

	"github.com/autoimport/crm/internal/store"
)

var page = template.Must(template.New("page").Parse(pageHTML))

const pageHTML = `<!DOCTYPE html>
<html lang="ru">
<head>
  <meta charset="utf-8">
  <title>{{.Title}} · сделка № {{.DealNumber}}</title>
  <style>
    @page { size: A4; margin: 16mm; }
    body { margin: 0; color: #16120c; background: #fff; font: 12pt/1.4 "Times New Roman", Times, serif; }
    h1 { font-size: 16pt; font-weight: 700; text-align: center; margin: 0 0 6pt; }
    h2 { font-size: 12pt; margin: 16pt 0 6pt; }
    .kicker { text-align: center; font-size: 10pt; letter-spacing: .12em; text-transform: uppercase; color: #5c564c; margin-bottom: 14pt; }
    .meta { display: flex; justify-content: space-between; font-size: 10pt; margin-bottom: 14pt; }
    p { margin: 0 0 8pt; text-align: justify; }
    table { width: 100%; border-collapse: collapse; margin: 8pt 0 12pt; }
    th, td { border: 1px solid #16120c; padding: 5pt 7pt; vertical-align: top; text-align: left; }
    th { width: 34%; font-weight: 400; color: #5c564c; }
    .sign { display: flex; justify-content: space-between; gap: 24pt; margin-top: 28pt; }
    .sign div { flex: 1; }
    .line { border-bottom: 1px solid #16120c; height: 28pt; margin-top: 18pt; }
    .note { font-size: 9pt; color: #5c564c; margin-top: 18pt; }
    .center { text-align: center; }
  </style>
</head>
<body>
  <p class="kicker">Импорт CN · JP · сделка № {{.DealNumber}}</p>
  <h1>{{.Title}}</h1>
  <div class="meta">
    <span>{{.Date}}</span>
    <span>Этап: {{.Stage}}</span>
  </div>
  {{if eq .Kind "contract"}}{{template "contract" .}}{{end}}
  {{if eq .Kind "invoice"}}{{template "invoice" .}}{{end}}
  {{if eq .Kind "payment_order"}}{{template "payment" .}}{{end}}
  {{if eq .Kind "customs_declaration"}}{{template "customs" .}}{{end}}
  {{if eq .Kind "vehicle_certificate"}}{{template "vehicle" .}}{{end}}
  {{if eq .Kind "acceptance_act"}}{{template "acceptance" .}}{{end}}
  {{if eq .Kind "passport"}}{{template "passport" .}}{{end}}
  <div class="sign">
    <div>Исполнитель<br>{{.Dealer.Legal}}<div class="line"></div>подпись / печать</div>
    <div>Заказчик<br>{{.Client.Name}}<div class="line"></div>подпись</div>
  </div>
  <p class="note">Форма заполнена из карточки сделки. Пустые поля отмечены как «не указано». Документ не заменяет нотариальное удостоверение и официальную таможенную декларацию.</p>
</body>
</html>

{{define "contract"}}
<p>г. {{.Dealer.City}}, {{.Date}}</p>
<p><b>{{.Dealer.Legal}}</b>, ИНН {{.Dealer.INN}}, адрес {{.Dealer.Address}}, тел. {{.Dealer.Phone}}, именуемое в дальнейшем «Исполнитель», и <b>{{.Client.Name}}</b>, паспорт {{.Client.Passport}}, адрес {{.Client.Address}}, тел. {{.Client.Phone}}, e-mail {{.Client.Email}}, именуемый в дальнейшем «Заказчик», заключили настоящий договор о нижеследующем.</p>
<h2>1. Предмет</h2>
<p>Исполнитель обязуется оказать услуги по подбору, выкупу, доставке и таможенному оформлению автомобиля, а Заказчик — принять и оплатить услуги. Наименование: {{.DealTitle}}.</p>
<table>
  <tr><th>Автомобиль</th><td>{{.Car.Title}}</td></tr>
  <tr><th>VIN</th><td>{{.Car.VIN}}</td></tr>
  <tr><th>Рынок</th><td>{{.Car.Origin}}</td></tr>
  <tr><th>Пробег / кузов</th><td>{{.Car.Mileage}} / {{.Car.Color}}</td></tr>
  <tr><th>Поставщик</th><td>{{.Seller}}</td></tr>
  <tr><th>Состав услуг</th><td>{{.Services}}</td></tr>
</table>
<h2>2. Цена и расчёты</h2>
<p>Стоимость по договору составляет {{.Amount}} ({{.AmountWords}}). Оплачено на дату формы: {{.Paid}} ({{.PaidShare}}). Остаток: {{.Remainder}}.</p>
<p>Оплата производится по счёту Исполнителя. Срок ориентировочной выдачи: {{.Handover}}.</p>
<h2>3. Порядок</h2>
<p>Стороны ведут сделку на площадке по этапам: лид, потребность, договор, оплата, привоз, растаможка, выдача. Текущий этап — {{.Stage}}. Сделка создана {{.Created}}.</p>
<p>Примечание менеджера: {{.Note}}</p>
<h2>4. Ответственность</h2>
<p>Исполнитель не отвечает за скрытые дефекты, не указанные в аукционном листе или экспортной карточке, если Заказчик согласился с лотом. Заказчик подтверждает достоверность паспортных данных.</p>
{{end}}

{{define "invoice"}}
<p class="center">Счёт № {{.DealNumber}} от {{.Date}}</p>
<table>
  <tr><th>Исполнитель</th><td>{{.Dealer.Legal}}<br>ИНН {{.Dealer.INN}}<br>{{.Dealer.Address}}<br>{{.Dealer.Phone}}, {{.Dealer.Email}}</td></tr>
  <tr><th>Заказчик (плательщик)</th><td>{{.Client.Name}}<br>паспорт {{.Client.Passport}}<br>{{.Client.Address}}<br>{{.Client.Phone}}, {{.Client.Email}}</td></tr>
  <tr><th>Основание</th><td>Договор по сделке № {{.DealNumber}} · {{.DealTitle}}</td></tr>
</table>
<table>
  <tr><th>Наименование</th><td>Автомобиль / услуги импорта: {{.Car.Title}}</td></tr>
  <tr><th>VIN</th><td>{{.Car.VIN}}</td></tr>
  <tr><th>Сумма к оплате</th><td>{{.Amount}}</td></tr>
  <tr><th>Прописью</th><td>{{.AmountWords}}</td></tr>
  <tr><th>Уже оплачено</th><td>{{.Paid}}</td></tr>
  <tr><th>Остаток</th><td>{{.Remainder}}</td></tr>
</table>
<p>Счёт сформирован автоматически из карточки сделки. НДС не выделяется, если Исполнитель применяет спецрежим — уточняется в договоре.</p>
{{end}}

{{define "payment"}}
<p class="center">Платёжное поручение / квитанция к сделке № {{.DealNumber}}</p>
<table>
  <tr><th>Плательщик</th><td>{{.Client.Name}}, паспорт {{.Client.Passport}}, {{.Client.Address}}</td></tr>
  <tr><th>Получатель</th><td>{{.Dealer.Legal}}, ИНН {{.Dealer.INN}}, {{.Dealer.Address}}</td></tr>
  <tr><th>Назначение платежа</th><td>Оплата по сделке № {{.DealNumber}} ({{.DealTitle}}), VIN {{.Car.VIN}}</td></tr>
  <tr><th>Сумма</th><td>{{.Amount}} ({{.AmountWords}})</td></tr>
  <tr><th>Оплачено / остаток</th><td>{{.Paid}} / {{.Remainder}}</td></tr>
  <tr><th>Контакт получателя</th><td>{{.Dealer.Phone}}, {{.Dealer.Email}}</td></tr>
</table>
<p>Документ служит памяткой для перевода. Банковские реквизиты Исполнитель указывает отдельно, если они не внесены в профиль.</p>
{{end}}

{{define "customs"}}
<p class="center">Сведения для таможенного оформления</p>
<p>Форма готовит пакет данных для декларации на транспортное средство. Официальную ДТ подаёт таможенный представитель.</p>
<table>
  <tr><th>Декларант / получатель</th><td>{{.Client.Name}}<br>паспорт {{.Client.Passport}}<br>{{.Client.Address}}<br>{{.Client.Phone}}</td></tr>
  <tr><th>Импортёр (дилер)</th><td>{{.Dealer.Legal}}, ИНН {{.Dealer.INN}}<br>{{.Dealer.Address}}</td></tr>
  <tr><th>Страна происхождения</th><td>{{.Car.Origin}}</td></tr>
  <tr><th>Марка / модель / год</th><td>{{.Car.Brand}} / {{.Car.Model}} / {{.Car.Year}}</td></tr>
  <tr><th>VIN</th><td>{{.Car.VIN}}</td></tr>
  <tr><th>Объём / мощность</th><td>{{.Car.Engine}} / {{.Car.Power}}</td></tr>
  <tr><th>Топливо / КПП / руль</th><td>{{.Car.Fuel}} / {{.Car.Gearbox}} / {{.Car.Steering}}</td></tr>
  <tr><th>Пробег / цвет</th><td>{{.Car.Mileage}} / {{.Car.Color}}</td></tr>
  <tr><th>Поставщик</th><td>{{.Seller}}</td></tr>
  <tr><th>Аукцион / лот</th><td>{{.Car.Auction}}</td></tr>
  <tr><th>Таможенная стоимость (ориентир)</th><td>{{.Amount}}</td></tr>
</table>
{{end}}

{{define "vehicle"}}
<p class="center">Карточка транспортного средства</p>
<table>
  <tr><th>Сделка</th><td>№ {{.DealNumber}} · {{.DealTitle}}</td></tr>
  <tr><th>Наименование</th><td>{{.Car.Title}}</td></tr>
  <tr><th>Марка</th><td>{{.Car.Brand}}</td></tr>
  <tr><th>Модель</th><td>{{.Car.Model}}</td></tr>
  <tr><th>Год</th><td>{{.Car.Year}}</td></tr>
  <tr><th>VIN</th><td>{{.Car.VIN}}</td></tr>
  <tr><th>Рынок</th><td>{{.Car.Origin}}</td></tr>
  <tr><th>Пробег</th><td>{{.Car.Mileage}}</td></tr>
  <tr><th>Двигатель</th><td>{{.Car.Engine}}, {{.Car.Power}}, {{.Car.Fuel}}</td></tr>
  <tr><th>КПП / привод / руль</th><td>{{.Car.Gearbox}} / {{.Car.Steering}}</td></tr>
  <tr><th>Цвет</th><td>{{.Car.Color}}</td></tr>
  <tr><th>Аукцион</th><td>{{.Car.Auction}}</td></tr>
  <tr><th>Собственник (заказчик)</th><td>{{.Client.Name}}, паспорт {{.Client.Passport}}</td></tr>
  <tr><th>Адрес собственника</th><td>{{.Client.Address}}</td></tr>
</table>
{{end}}

{{define "acceptance"}}
<p class="center">Акт приёма-передачи № {{.DealNumber}}</p>
<p>г. {{.Dealer.City}}, {{.Date}}</p>
<p>{{.Dealer.Legal}} передаёт, а {{.Client.Name}} принимает автомобиль по сделке № {{.DealNumber}}.</p>
<table>
  <tr><th>Автомобиль</th><td>{{.Car.Title}}</td></tr>
  <tr><th>VIN</th><td>{{.Car.VIN}}</td></tr>
  <tr><th>Пробег на выдаче</th><td>{{.Car.Mileage}}</td></tr>
  <tr><th>Цвет / руль</th><td>{{.Car.Color}} / {{.Car.Steering}}</td></tr>
  <tr><th>Комплект документов</th><td>договор, счёт, сведения для таможни, карточка ТС, паспортные данные заказчика</td></tr>
  <tr><th>Оплачено</th><td>{{.Paid}} из {{.Amount}}</td></tr>
  <tr><th>План выдачи</th><td>{{.Handover}}</td></tr>
</table>
<p>Заказчик подтверждает, что осмотрел автомобиль, претензий по комплектности на момент передачи не имеет либо фиксирует их ниже.</p>
<p>Замечания: _______________________________________________________________</p>
{{end}}

{{define "passport"}}
<p class="center">Анкета заказчика (паспортные данные)</p>
<p>Данные подставляются из профиля клиента. Скан паспорта при необходимости прикрепляется отдельно.</p>
<table>
  <tr><th>ФИО</th><td>{{.Client.Name}}</td></tr>
  <tr><th>Паспорт</th><td>{{.Client.Passport}}</td></tr>
  <tr><th>Адрес регистрации</th><td>{{.Client.Address}}</td></tr>
  <tr><th>Телефон</th><td>{{.Client.Phone}}</td></tr>
  <tr><th>Электронная почта</th><td>{{.Client.Email}}</td></tr>
  <tr><th>Сделка</th><td>№ {{.DealNumber}} · {{.DealTitle}}</td></tr>
  <tr><th>Автомобиль</th><td>{{.Car.Title}}, VIN {{.Car.VIN}}</td></tr>
</table>
<p>Заказчик подтверждает полноту и достоверность сведений и соглашается на их использование для договора, оплаты и таможенного оформления данной сделки.</p>
{{end}}
`

// Render возвращает HTML печатной формы указанного вида.
func Render(kind store.DocumentKind, payload Payload) ([]byte, error) {
	if kind == store.DocOther {
		return nil, fmt.Errorf("вид «прочее» только загружается сканом")
	}
	if !kind.Valid() {
		return nil, fmt.Errorf("неизвестный вид документа")
	}
	payload.Kind = string(kind)
	payload.Title = kind.Title()

	var buf bytes.Buffer
	if err := page.Execute(&buf, payload); err != nil {
		return nil, fmt.Errorf("сборка формы %s: %w", kind, err)
	}
	html := buf.Bytes()
	if !strings.Contains(string(html), payload.DealNumber) {
		return nil, fmt.Errorf("форма не содержит номер сделки")
	}
	return html, nil
}

// PrintableKinds — виды, которые система умеет заполнить сама.
func PrintableKinds() []store.DocumentKind {
	return []store.DocumentKind{
		store.DocContract,
		store.DocInvoice,
		store.DocPayment,
		store.DocCustoms,
		store.DocCertificate,
		store.DocAcceptance,
		store.DocPassport,
	}
}
