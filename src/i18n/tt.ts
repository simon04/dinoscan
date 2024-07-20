import { useFetch } from "@vueuse/core";
import { computed, ref } from "vue";
import type en from "./en.json";

type I18nKey = keyof typeof en;

export const language = ref("en");

const { data } = useFetch(
  "https://tools-static.wmflabs.org/tooltranslate/data/petscan/toolinfo.json",
).json<{ languages: string[] }>();
export const languages = computed(() => data.value?.languages || []);

const { data: messages } = useFetch(
  computed(
    () =>
      `https://tools-static.wmflabs.org/tooltranslate/data/petscan/${language.value}.json`,
  ),
).json<Record<I18nKey, string>>();

export default function tt(key: I18nKey): string {
  return messages.value?.[key]?.replace("<br/>", "\n") || key;
}
