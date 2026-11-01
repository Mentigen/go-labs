# Мутационное тестирование

Проверены `internal/domain` и `internal/usecase` с помощью [gremlins v0.6.0](https://github.com/go-gremlins/gremlins). Включены все мутаторы, в том числе необязательные.

```
gremlins unleash ./internal/domain  --invert-assignments --invert-bitwise --invert-logical \
    --invert-loopctrl --remove-self-assignments --timeout-coefficient 30
gremlins unleash ./internal/usecase --invert-assignments --invert-bitwise --invert-logical \
    --invert-loopctrl --remove-self-assignments --timeout-coefficient 30
```

Установка: `go install github.com/go-gremlins/gremlins/cmd/gremlins@v0.6.0`. Запуск через `make mutation` (`$(go env GOPATH)/bin` должен входить в `PATH`). Повторный прогон 30.10.2026 подтвердил числа в таблице; таймаутов и некомпилируемых мутантов нет.

## Отчёт

В `internal/domain` всего 26 мутантов: 22 убитых, 0 выживших, 4 непокрытых. Efficacy составляет 100%.

В `internal/usecase` всего 20 мутантов: 19 убитых, 0 выживших, 1 непокрытых. Efficacy составляет 100%.

Efficacy это доля убитых среди убитых и выживших. Мутанты "не покрыто" в `gremlins` это мутации констант: объявление константы не считается исполняемой строкой, поэтому инструмент их не запускает. Эти мутации проверены вручную, результаты описаны ниже.

## Разбор

Мутанты `TIMED OUT`. При таймауте по умолчанию 18 мутантов в `internal/domain` заканчивались таймаутом: `gremlins` берёт лимит из времени сбора покрытия, а запуск `go test` с компиляцией занимает дольше. Это ограничение настройки, а не слабость тестов. С `--timeout-coefficient 30` все 18 убиты.

Мутации констант в `domain/trip/trip.go` (4 мутанта: `minLatitude = -90` и `minLongitude = -180`, замена знака и арифметики). Подставлялись вручную значения `90`, `180`, `-91`, `-181` и запускались тесты домена. Все четыре убиты тестом `TestNewBoundaryValuesAreValid` (граничные значения -90 и -180 должны быть допустимыми) и таблицей `TestNewValidation`.

Мутация константы `idempotencyKeyTTL = 24 * time.Hour` в `usecase/trip/create/usecase.go`. До правок тесты передавали TTL в мок через `gomock.Any()`. Ручная проверка показала: мутанты `24 / time.Hour` (TTL равен нулю, все ключи сразу протухают) и `23 * time.Hour` выживали. Изменение TTL не ловилось ни одним тестом. После правок мок получает точное значение `24 * time.Hour` в вызовах `GetIdempotencyKey` и `ClaimIdempotencyKey`, и те же мутанты (а также `24 + time.Hour`) убиваются.

## До и после

Выжившие мутанты констант TTL: до исправлений 2 из 2 проверенных (`24 / time.Hour`, `23 * time.Hour`), после 0 из 3 (добавлен `24 + time.Hour`).

`internal/usecase`: убито / выжило / не покрыто: до исправлений 19 / 0 / 1, после 19 / 0 / 1.

`internal/domain`: убито / выжило / не покрыто: до исправлений 22 / 0 / 4, после 22 / 0 / 4.

Число в строках `gremlins` не меняется, потому что инструмент не запускает мутанты констант. Разница видна только в ручной проверке, поэтому она приведена отдельно.

## Мутанты, которые нельзя убить

Таких среди мутантов `gremlins` в этих пакетах нет: каждый запущенный мутант убит.
