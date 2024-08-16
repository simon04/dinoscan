import { useFetch, useLocalStorage } from "@vueuse/core";
import { computed } from "vue";
import type en from "./en.json";

type I18nKey = keyof typeof en;

export const language = useLocalStorage("dinoscan.language", "en");

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
  return message?.replace(/<br\/?>/, "\n") || key;
}

// https://github.com/i18next/i18next/blob/d7fcec65faf4cb8f232a721d481e0d29ed684159/src/i18next.js#L513
const rtlLngs = [
  "ar",
  "shu",
  "sqr",
  "ssh",
  "xaa",
  "yhd",
  "yud",
  "aao",
  "abh",
  "abv",
  "acm",
  "acq",
  "acw",
  "acx",
  "acy",
  "adf",
  "ads",
  "aeb",
  "aec",
  "afb",
  "ajp",
  "apc",
  "apd",
  "arb",
  "arq",
  "ars",
  "ary",
  "arz",
  "auz",
  "avl",
  "ayh",
  "ayl",
  "ayn",
  "ayp",
  "bbz",
  "pga",
  "he",
  "iw",
  "ps",
  "pbt",
  "pbu",
  "pst",
  "prp",
  "prd",
  "ug",
  "ur",
  "ydd",
  "yds",
  "yih",
  "ji",
  "yi",
  "hbo",
  "men",
  "xmn",
  "fa",
  "jpr",
  "peo",
  "pes",
  "prs",
  "dv",
  "sam",
  "ckb",
];

// https://github.com/i18next/i18next/blob/d7fcec65faf4cb8f232a721d481e0d29ed684159/src/i18next.js#L580
export const isRTL = computed(
  () => rtlLngs.includes(language.value) || language.value.endsWith("-arab"),
);
