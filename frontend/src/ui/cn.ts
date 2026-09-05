/**
 * Сборка списка классов.
 *
 * Без внешней зависимости: задача — отбросить пустые значения, а разрешение
 * конфликтов утилит (`p-2` против `p-4`) здесь не нужно, потому что базовые
 * классы компонентов и переопределения из props не пересекаются по свойствам.
 */
export type ClassValue = string | false | null | undefined;

export function cn(...values: ClassValue[]): string {
  let result = '';
  for (const value of values) {
    if (!value) continue;
    result = result ? `${result} ${value}` : value;
  }
  return result;
}
