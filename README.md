### Hexlet tests and linter status:
[![Actions Status](https://github.com/misterDillett/go-project-316/actions/workflows/hexlet-check.yml/badge.svg)](https://github.com/misterDillett/go-project-316/actions)

## Глубина обхода

Параметр `--depth` (или `Depth` в структуре Options) определяет максимальную глубину перехода по ссылкам внутри исходного домена:

- `depth = 0` - анализируется только стартовая страница
- `depth = 1` - стартовая страница и все страницы, на которые есть прямые ссылки
- `depth = 2` - дополнительно анализируются страницы, найденные на глубине 1

### Примеры

```bash
# Анализ только главной страницы (без перехода по ссылкам)
./hexlet-go-crawler --depth 0 https://example.com

# Анализ главной страницы и всех прямых ссылок
./hexlet-go-crawler --depth 1 https://example.com

# Глубокий анализ с задержкой между запросами
./hexlet-go-crawler --depth 3 --delay 500ms https://example.com

# Анализ с повторными попытками при ошибках
./hexlet-go-crawler --retries 3 https://example.com

# Компактный JSON (без отступов)
./hexlet-go-crawler --indent=false https://example.com

# Комбинация параметров
./hexlet-go-crawler --depth 2 --delay 1s --retries 2 --timeout 30s https://example.com
