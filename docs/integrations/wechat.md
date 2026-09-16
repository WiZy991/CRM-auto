# WeChat（微信公众号）

Интеграция с **WeChat Official Account** (公众号 / 服务号): дилер сохраняет AppID и AppSecret своего кабинета, CRM проверяет связь через `access_token` и может класть лот в **черновики** (草稿箱).

Личный WeChat и Moments (朋友圈) **не** поддерживаются публичным API для автопостинга объявлений — нужен именно кабинет [mp.weixin.qq.com](https://mp.weixin.qq.com/).

## Что умеет CRM сейчас

| Действие | Статус |
|----------|--------|
| Сохранение AppID + AppSecret | да |
| «Проверить связь» (`cgi-bin/token`) | да, реальный вызов |
| Автопост лота | черновик в draft box (`draft/add`) |
| Автопубликация в ленту (`freepublish`) | нет — квота жёсткая, дилер публикует из MP |

## Как получить ключи

1. Зарегистрируйте или войдите в [微信公众平台](https://mp.weixin.qq.com/) (服务号 предпочтительнее для API).
2. Пройдите верификацию субъекта (企业认证), если планируете публикацию — без неё часть методов закрыта.
3. **设置与开发 → 基本配置** (Settings & Development → Basic Configuration):
   - скопируйте **AppID**;
   - сгенерируйте **AppSecret** (показывается один раз).
4. На той же странице добавьте **IP whitelist**: исходящий IP сервера CRM (VPS). Без whitelist WeChat отвечает `40164`.
5. В CRM: **Каналы → WeChat** → AppID + AppSecret → **Сохранить** → **Проверить связь**.

Тестовый аккаунт для разработки: [WeChat MP test account](https://mp.weixin.qq.com/debug/cgi-bin/sandboxinfo) (ограничения по сравнению с боевым 服务号).

## Как устроен API (кратко)

1. `GET /cgi-bin/token?grant_type=client_credential&appid=&secret=` → `access_token` (~2 часа, лимит ~2000 вызовов/сутки на AppID).
2. Загрузка обложки: `POST /cgi-bin/material/add_material?type=image`.
3. Черновик: `POST /cgi-bin/draft/add` с `title`, `thumb_media_id`, HTML `content`.
4. Публикация вручную в кабинете MP или позже через `freepublish/submit` (отдельный этап).

Официальная документация: [developers.weixin.qq.com](https://developers.weixin.qq.com/doc/offiaccount/Getting_Started/Overview.html).

## Ограничения

- Картинки в статье должны быть из media WeChat — чужие URL в `content` отфильтруются.
- Заголовок ~32 иероглифа / короткая строка.
- Групповая рассылка и freepublish: жёсткие квоты (服务号 ≈ 4/мес на mass send; freepublish — отдельно, нужна авторизация).
- Сервер CRM должен ходить в `api.weixin.qq.com` (из РФ/РФ-прокси иногда нужен whitelist и стабильный исходящий IP).
- WeChat Work（企业微信）— другой продукт и другие credentials; в этой версии не подключен.

## Поля в CRM

| Поле | Что это |
|------|---------|
| AppID | `client_id` |
| AppSecret | `client_secret` / token |

## Типичные ошибки

| Код / симптом | Что делать |
|---------------|------------|
| `40164` | Добавить IP сервера в whitelist MP |
| `40125` / `40013` | Неверный AppSecret или AppID |
| `40001` / `42001` | Просроченный token — нажать «Проверить связь» снова |
| Нет фото у лота | Черновик не создаётся: нужна обложка |
| Черновик есть, в ленте нет | Откройте 草稿箱 в MP и опубликуйте вручную |

## Что появится дальше

- Опциональный `freepublish/submit` по флагу дилера.
- Несколько фото / карусель в content через `uploadimg`.
- Кэш `access_token` в Redis (сейчас токен запрашивается на каждый Test/Publish).
