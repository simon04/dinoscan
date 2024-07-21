import { useFetch } from "@vueuse/core";
import { computed, ref } from "vue";
import type en from "./en.json";

type I18nKey = keyof typeof en;

export const language = ref("en");

const { data: languages0 } = useFetch(
  "https://tools-static.wmflabs.org/tooltranslate/data/languages.json",
).json<Record<string, string>>();

const { data: toolinfo } = useFetch(
  "https://tools-static.wmflabs.org/tooltranslate/data/petscan/toolinfo.json",
).json<{ languages: string[] }>();

export const languages = computed(
  () =>
    toolinfo.value?.languages.map((l) => ({
      value: l,
      label: `${l} ${languages0.value?.[l] || ""}`,
    })) || [],
);

const { data: messagesEN } = useFetch(
  "https://tools-static.wmflabs.org/tooltranslate/data/petscan/en.json",
).json<Record<I18nKey, string>>();

const { data: messages } = useFetch(
  computed(
    () =>
      `https://tools-static.wmflabs.org/tooltranslate/data/petscan/${language.value}.json`,
  ),
  { refetch: true },
).json<Record<I18nKey, string>>();

export default function tt(key: I18nKey): string {
  const message = messages.value?.[key] || messagesEN.value?.[key];
  return message?.replace("<br/>", "\n") || key;
}
