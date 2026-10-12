"use strict";
const $ = (id) => document.getElementById(id);
const money = (value) =>
  new Intl.NumberFormat("ru-RU", {
    style: "currency",
    currency: "KZT",
    currencyDisplay: "narrowSymbol",
    minimumFractionDigits: 0,
    maximumFractionDigits: 2,
  }).format(value);
let timezone = "Asia/Almaty";
let today = "";
let requestNumber = 0;
async function api(path, options) {
  const response = await fetch(path, options);
  const data = await response.json();
  if (!response.ok)
    throw new Error(data.error?.message || "Не удалось выполнить запрос.");
  return data;
}
function notify(text, error = false) {
  $("message").textContent = text;
  $("message").className = error ? "error" : "";
  $("message").hidden = !text;
}
function localParts(date) {
  const parts = new Intl.DateTimeFormat("sv-SE", {
    timeZone: timezone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hourCycle: "h23",
  }).formatToParts(date);
  const p = Object.fromEntries(parts.map((x) => [x.type, x.value]));
  return `${p.year}-${p.month}-${p.day}T${p.hour}:${p.minute}:${p.second}`;
}
// Interpret form wall time in the configured reporting zone, independent of the browser zone.
function zonedISO(value) {
  const wall = Date.parse(value + ":00Z");
  let instant = wall;
  for (let i = 0; i < 4; i++)
    instant += wall - Date.parse(localParts(new Date(instant)) + "Z");
  if (localParts(new Date(instant)).slice(0, 16) !== value)
    throw new Error(
      "Это местное время не существует из-за перевода часов. Выберите другое время.",
    );
  return new Date(instant).toISOString();
}
const time = (value) =>
  new Intl.DateTimeFormat("ru-RU", {
    timeZone: timezone,
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(value));
function cell(row, text, className) {
  const td = document.createElement("td");
  td.textContent = text;
  if (className) td.className = className;
  row.append(td);
  return td;
}
function render(data) {
  const s = data.summary;
  for (const key of ["net", "revenue", "commission"])
    $(key).textContent = money(s[key]);
  $("count").textContent = s.count;
  $("trip-count").textContent = `Всего: ${s.count}`;
  $("payment-total").textContent = money(s.revenue);
  $("cash").textContent = money(s.cash.revenue);
  $("card").textContent = money(s.card.revenue);
  $("cash-bar").style.flexGrow = s.cash.revenue;
  $("card-bar").style.flexGrow = s.card.revenue;
  $("day-caption").textContent = new Intl.DateTimeFormat("ru-RU", {
    day: "numeric",
    month: "long",
    year: "numeric",
    timeZone: "UTC",
  }).format(new Date(data.date + "T12:00:00Z"));
  const minutes = data.trips.reduce(
    (sum, t) =>
      sum + Math.round((Date.parse(t.end) - Date.parse(t.start)) / 60000),
    0,
  );
  $("duration").textContent = minutes
    ? `${Math.floor(minutes / 60)} ч ${minutes % 60} мин в поездках`
    : "За выбранный день";
  $("trips").replaceChildren();
  for (const trip of data.trips) {
    const row = document.createElement("tr");
    const td = cell(
      row,
      `${time(trip.start)} — ${time(trip.end)}`,
      "trip-time",
    );
    const duration = document.createElement("small");
    duration.textContent = `${Math.round((Date.parse(trip.end) - Date.parse(trip.start)) / 60000)} мин`;
    td.append(duration);
    const payment = cell(row, "");
    const badge = document.createElement("span");
    badge.className = `badge ${trip.payment}`;
    badge.textContent = trip.payment === "cash" ? "Наличные" : "Карта";
    payment.append(badge);
    cell(row, money(trip.amount), "number");
    cell(row, money(trip.commission), "number");
    cell(
      row,
      money(
        (Math.round(trip.amount * 100) - Math.round(trip.commission * 100)) /
          100,
      ),
      "number net-cell",
    );
    $("trips").append(row);
  }
  $("empty").hidden = data.trips.length > 0;
}
async function loadDay() {
  const request = ++requestNumber;
  $("report").setAttribute("aria-busy", "true");
  try {
    const data = await api(
      `/api/day?date=${encodeURIComponent($("date").value)}`,
    );
    if (request !== requestNumber) return;
    render(data);
    $("report").hidden = false;
    notify("");
  } catch (error) {
    if (request === requestNumber) {
      $("report").hidden = true;
      notify("Не удалось загрузить день. " + error.message, true);
    }
  } finally {
    if (request === requestNumber)
      $("report").setAttribute("aria-busy", "false");
  }
}
function moveDay(delta) {
  const date = new Date($("date").value + "T12:00:00Z");
  if (!Number.isFinite(date.getTime())) return;
  date.setUTCDate(date.getUTCDate() + delta);
  $("date").value = date.toISOString().slice(0, 10);
  loadDay();
}
function openForm() {
  $("trip-form").reset();
  $("form-error").hidden = true;
  const date = $("date").value || today;
  $("trip-form").elements.start.value = date + "T08:00";
  $("trip-form").elements.end.value = date + "T08:30";
  $("form-zone").textContent = `Местное время: ${timezone}`;
  $("trip-dialog").showModal();
}
$("previous").addEventListener("click", () => moveDay(-1));
$("next").addEventListener("click", () => moveDay(1));
$("date").addEventListener("change", loadDay);
$("today").addEventListener("click", () => {
  $("date").value = today;
  loadDay();
});
for (const id of ["add-trip", "add-empty"])
  $(id).addEventListener("click", openForm);
for (const id of ["close-dialog", "cancel-dialog"])
  $(id).addEventListener("click", () => $("trip-dialog").close());
$("trip-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  $("form-error").hidden = true;
  const f = event.currentTarget.elements;
  try {
    const trip = {
      start: zonedISO(f.start.value),
      end: zonedISO(f.end.value),
      amount: Number(f.amount.value),
      payment: f.payment.value,
      commission: Number(f.commission.value),
    };
    if (Date.parse(trip.end) <= Date.parse(trip.start))
      throw new Error("Окончание поездки должно быть позже начала.");
    if (trip.commission > trip.amount)
      throw new Error("Комиссия не может превышать сумму поездки.");
    $("save-trip").disabled = true;
    $("save-trip").textContent = "Сохраняем…";
    const result = await api("/api/trips", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(trip),
    });
    $("date").value = f.start.value.slice(0, 10);
    $("trip-dialog").close();
    await loadDay();
    if (!$("report").hidden)
      notify(
        result.created
          ? "Поездка добавлена. Сводка обновлена."
          : "Эта поездка уже есть в дневнике. Дубликат не создан.",
      );
  } catch (error) {
    $("form-error").textContent = error.message;
    $("form-error").hidden = false;
  } finally {
    $("save-trip").disabled = false;
    $("save-trip").textContent = "Сохранить поездку";
  }
});
(async () => {
  try {
    const data = await api("/api/days");
    timezone = data.timezone;
    today = data.today;
    $("timezone").textContent = `Время: ${timezone}`;
    $("date").value = data.days[0] || today;
    await loadDay();
  } catch (error) {
    $("report").hidden = true;
    notify(
      "Не удалось подключиться к серверу. Обновите страницу. " + error.message,
      true,
    );
  }
})();
