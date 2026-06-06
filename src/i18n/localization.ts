import { defaultLocale, langProps } from "./props";

import i18n, { Resource } from "i18next";

const STORAGE_KEY = "decky-clash-language";

export class localizationManager {
  private static language = "english";

  public static async init() {
    const resources: Resource = Object.keys(langProps).reduce(
      (acc, key) => {
        acc[langProps[key].locale] = {
          translation: langProps[key].strings,
        };
        return acc;
      },
      {} as Resource
    );

    this.language = await this.resolveLanguage();
    console.log(
      `[GeekcomClash] language: stored="${this.getStored()}" -> "${this.language}" (locale ${this.getLocale()})`
    );

    i18n.init({
      resources: resources,
      lng: this.getLocale(),
      fallbackLng: defaultLocale,
      returnEmptyString: false,
      interpolation: {
        escapeValue: false,
      },
    });
  }

  // Текущий сохранённый выбор: "auto" | ключ языка ("russian"/"english"/...)
  public static getStored(): string {
    return window.localStorage.getItem(STORAGE_KEY) || "auto";
  }

  // Сменить язык в рантайме: сохранить выбор и применить.
  public static async setLanguage(choice: string): Promise<void> {
    window.localStorage.setItem(STORAGE_KEY, choice);
    this.language = await this.resolveLanguage();
    await i18n.changeLanguage(this.getLocale());
  }

  // Опции для выпадающего списка: Auto + все доступные языки (нативные названия).
  public static getLanguageOptions(): { key: string; label: string }[] {
    return Object.keys(langProps).map((k) => ({
      key: k,
      label: langProps[k].label,
    }));
  }

  // Выбор пользователя имеет приоритет; "auto"/невалидный -> язык Steam.
  private static async resolveLanguage(): Promise<string> {
    const stored = this.getStored();
    if (stored !== "auto" && langProps[stored]) {
      return stored;
    }
    const raw = (await SteamClient.Settings.GetCurrentLanguage()) || "english";
    return String(raw).toLowerCase();
  }

  private static getLocale() {
    return langProps[this.language]?.locale ?? defaultLocale;
  }
}
