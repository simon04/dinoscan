import { useFetch } from "@vueuse/core";
import { computed } from "vue";

const { data } = await useFetch(
  "https://commons.wikimedia.org/w/api.php?origin=*&action=sitematrix&smtype=language&format=json&formatversion=2",
).json<{ sitematrix: Sitematrix }>();

const matrix = computed((): Matrix[] =>
  Object.values(data.value?.sitematrix || {}).filter(
    (v) => typeof v === "object",
  ),
);
const languages = computed(() => matrix.value.map((m) => m.code));

export function useSiteMatrix() {
  return { matrix, languages };
}

type Sitematrix = Record<string, Matrix> & { count: number };

interface Matrix {
  code: string;
  name: string;
  site: Site[];
  dir: Dir;
  localname: string;
}

enum Dir {
  LTR = "ltr",
  RTL = "rtl",
}

interface Site {
  url: string;
  dbname: string;
  code: Code;
  sitename: string;
  closed?: boolean;
}

enum Code {
  Wiki = "wiki",
  Wikibooks = "wikibooks",
  Wikinews = "wikinews",
  Wikiquote = "wikiquote",
  Wikisource = "wikisource",
  Wikiversity = "wikiversity",
  Wikivoyage = "wikivoyage",
  Wiktionary = "wiktionary",
}
