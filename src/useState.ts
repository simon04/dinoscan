import { reactive } from "vue";

export function useState() {
  return state;
}

const state = reactive({
  active_tab: "tab_categories",
  add_coordinates: undefined as true | undefined,
  add_defaultsort: undefined as true | undefined,
  add_disambiguation: undefined as true | undefined,
  add_image: undefined as true | undefined,
  article_redlinks_only: undefined as true | undefined,
  after: "",
  before: "",
  categories: "",
  cb_labels_any_l: 1,
  cb_labels_no_l: 1,
  cb_labels_yes_l: 1,
  combination: "subset",
  common_wiki_other: "",
  common_wiki: "auto",
  depth: 0,
  doit: "Do it!",
  "edits[anons]": "both",
  "edits[bots]": "both",
  "edits[flagged]": "both",
  ext_image_data: undefined as true | undefined,
  file_usage_data: undefined as true | undefined,
  file_usage_data_ns0: undefined as true | undefined,
  format: "json",
  interface_language: "en",
  "json-pretty": undefined as true | undefined,
  labels_any: "",
  labels_no: "",
  labels_yes: "",
  langs_labels_any: "",
  langs_labels_no: "",
  langs_labels_yes: "",
  language: "en",
  larger: "",
  links_to_all: "",
  links_to_any: "",
  links_to_no: "",
  manual_list_wiki: "",
  manual_list: "",
  max_age: "",
  max_sitelink_count: "",
  maxlinks: "",
  min_redlink_count: 1,
  min_sitelink_count: "",
  minlinks: "",
  namespace_conversion: "keep",
  negcats: "",
  "ns[0]": true,
  "ns[6]": false,
  ores_prediction: "any",
  ores_prob_from: "",
  ores_prob_to: "",
  ores_type: "any",
  outlinks_any: "",
  outlinks_no: "",
  outlinks_yes: "",
  output_compatability: "catscan",
  output_limit: "",
  page_image: "any",
  pagepile: "",
  project: "wikipedia",
  referrer_name: "",
  referrer_url: "",
  remove_template_redlinks: undefined as true | undefined,
  rxp_filter: "",
  search_filter: "",
  search_max_results: 500,
  search_query: "",
  search_wiki: "",
  show_disambiguation_pages: "both",
  show_redirects: "both",
  show_redlinks: undefined as true | undefined,
  show_soft_redirects: "both",
  since_rev0: "",
  sitelinks_any: "",
  sitelinks_no: "",
  sitelinks_yes: "",
  smaller: "",
  sortby: "none",
  sortorder: "ascending",
  source_combination: "",
  sparql: "",
  sparse: undefined as true | undefined,
  subpage_filter: "either",
  templates_any: "",
  templates_no: "",
  templates_use_talk_any: undefined as true | undefined,
  templates_use_talk_no: undefined as true | undefined,
  templates_use_talk_yes: undefined as true | undefined,
  templates_yes: "",
  thumbnails_in_wiki_output: undefined as true | undefined,
  wikidata_item: "no",
  wikidata_label_language: "",
  wikidata_prop_item_use: "",
  wikidata_source_sites: "",
  wpiu: "any",
});
