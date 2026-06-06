// Geekcom Clash — центральная точка кастомизации форка.
// Всё, что отличает наш форк от апстрима DeckyClash, держим здесь,
// чтобы при мердже апстрима конфликты были минимальны.

export const BRAND = {
  // Отображаемое имя плагина (заголовок панели, QAM, список плагинов).
  name: "Geekcom Clash",

  // Публичные ссылки для кнопок «Сообщество». Пустые не показываются.
  links: {
    boosty: "https://boosty.to/steamdecks",
    tgGames: "https://t.me/geekcom_deck_games",   // канал «Steam Deck Games»
    tgNews: "https://t.me/geekcomdeck_news",       // канал новостей
    tgChat: "https://t.me/Geekcom_hub",            // чат Geekcom-HUB
    github: "https://github.com/geekcom/geekcom-clash", // TODO: подставить реального владельца репо
  },
};
