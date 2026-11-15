import { EvidenceField, type Proposal } from "./extractions";

export type Variant = { id: string; name: string; code: string | null; price: number | null; attributes: Record<string, string>; attributeRows?: [string, string][]; priceKnown?: boolean };
export type Product = { id: string; name: string; revision: number; status: string; variants: Variant[] };
export type Source = { selectionId: string; postId: string; caption: string; proposal: Proposal | null };

export function productFields(product: Product) {
  return { name: product.name.trim(), variants: product.variants.map(({ attributeRows, priceKnown: _, ...v }) => ({ ...v, name: v.name.trim(), code: v.code?.trim().toUpperCase() || null, attributes: Object.fromEntries(attributeRows ?? Object.entries(v.attributes)) })) };
}
export function reviewErrors(product: Product): Record<string, string> {
  const errors: Record<string, string> = {};
  if (!product.name.trim()) errors.name = "نام محصول را کامل وارد کنید.";
  if (!product.variants.length) errors.variants = "حداقل یک تنوع ثبت کنید.";
  const codes = new Set<string>();
  product.variants.forEach((v, i) => {
    const prefix = `variants.${i}.`;
    if (!v.name.trim()) errors[prefix + "name"] = "نام تنوع را کامل وارد کنید.";
    const code = v.code?.trim().toUpperCase();
    if (code && codes.has(code)) errors[prefix + "code"] = "کد هر تنوع باید یکتا باشد.";
    if (code) codes.add(code);
    if ((v.priceKnown && v.price === null) || (v.price !== null && (!Number.isSafeInteger(v.price) || v.price < 0))) errors[prefix + "price"] = "مبلغ صحیح و نامنفی وارد کنید یا قیمت نامعلوم را انتخاب کنید.";
    const rows = v.attributeRows ?? Object.entries(v.attributes);
    if (rows.some(([key, value]) => !key.trim() || !value.trim())) errors[prefix + "attributes"] = "نام و مقدار ویژگی را کامل وارد کنید.";
    if (new Set(rows.map(([key]) => key)).size !== rows.length) errors[prefix + "attributes"] = "نام ویژگی‌ها تکراری است؛ برای هر ویژگی نام جداگانه وارد کنید.";
  });
  return errors;
}
const price = (v: Variant) => v.price === null ? "قیمت نامعلوم" : `${v.price.toLocaleString("fa-IR")} تومان`;
function fields(product: Product): [string, string][] {
  return [["نام محصول", product.name], ...product.variants.flatMap((v, i): [string, string][] => {
    const prefix = `تنوع ${(i + 1).toLocaleString("fa-IR")}`;
    return [[`${prefix}، نام`, v.name], [`${prefix}، کد`, v.code || "بدون کد"], [`${prefix}، قیمت`, price(v)], ...Object.entries(v.attributes).map(([k, value]): [string, string] => [`${prefix}، ویژگی ${k}`, value])];
  })];
}
export function ReviewSummary({ product, saved, dirty }: { product: Product; saved: Product | null; dirty: boolean }) {
  const current = new Map(fields({ ...product, ...productFields(product) }));
  const previous = new Map(saved ? fields(saved) : []);
  const labels = [...new Set([...previous.keys(), ...current.keys()])];
  return <section className="catalog-review" aria-label={dirty ? "خلاصهٔ تغییرات" : "خلاصهٔ پیش از تأیید"}>
    <h3>{dirty ? "پیش از ذخیره، تغییرات را بررسی کنید" : "خلاصهٔ پیش از تأیید"}</h3>
    {dirty && <p>پس از ذخیره، محصول را جداگانه تأیید کنید.</p>}
    <dl>{labels.filter(label => !dirty || previous.get(label) !== current.get(label)).map(label => <div key={label}><dt>{label}</dt><dd>{dirty && saved && <><span className="review-before">قبل: {previous.get(label) ?? "ثبت نشده"}</span><span>بعد: </span></>}{current.get(label) ?? "حذف می‌شود"}</dd></div>)}</dl>
  </section>;
}
export function CaptionSources({ sources }: { sources: Source[] }) {
  return <section className="catalog-sources" aria-label="شواهد کپشن">
    <h3>شواهد کپشن</h3>
    {sources.length === 0 ? <p>کپشن منبعی برای این محصول ثبت نشده است.</p> : sources.map(source => <details key={source.selectionId}><summary>کپشن و پیشنهاد اولیهٔ پست <bdi>{source.postId}</bdi></summary>
      <blockquote>{source.caption || "کپشن خالی است."}</blockquote>
      <p>این شواهد اولیه محفوظ می‌مانند؛ اصلاح شما در اطلاعات محصول و تاریخچه ثبت می‌شود.</p>
      {source.proposal && <dl><EvidenceField label="نام پیشنهادی" field={source.proposal.name} /><EvidenceField label="کد پیشنهادی" field={source.proposal.code} /><EvidenceField label="قیمت پیشنهادی" field={source.proposal.price} money />{Object.entries(source.proposal.attributes).map(([key, field]) => <EvidenceField key={key} label={key} field={field} />)}</dl>}
    </details>)}
  </section>;
}
