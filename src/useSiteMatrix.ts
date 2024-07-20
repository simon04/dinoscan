import { useFetch } from "@vueuse/core";
import { computed } from "vue";

const { data } = useFetch(
  "https://commons.wikimedia.org/w/api.php?action=sitematrix&smtype=language&format=json&formatversion=2",
).json<Sitematrix>();

export function useSiteMatrix() {
  const matrix = computed((): Matrix[] =>
    Object.values(data.value || {}).filter((v) => typeof v === "object"),
  );
  return { matrix };
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
