// Operational Admin master-data pages. Each is a thin configuration of the
// reusable CrudResource — columns to show, fields to edit — so all seven master
// tables get full create/edit/delete with almost no per-page code.
import CrudResource from "../../components/CrudResource";
import { Badge } from "../../components/ui";
import { opsApi } from "../../api/endpoints";

const yesNo = (v) => (v ? <Badge tone="open">yes</Badge> : <Badge tone="draft">no</Badge>);

export function EventTypesPage() {
  return (
    <CrudResource
      title="Event Types"
      subtitle="Championship tiers and their certificate branding."
      api={opsApi.eventTypes}
      columns={[
        { key: "code", label: "Code" },
        { key: "name", label: "Name" },
        { key: "certificateSeal", label: "Seal" },
        { key: "isActive", label: "Active", render: (r) => yesNo(r.isActive) },
      ]}
      fields={[
        { key: "code", label: "Code", hidden: true },
        { key: "name", label: "Name" },
        {
          key: "certificateSeal",
          label: "Certificate seal",
          type: "select",
          options: ["gold_hologram", "silver", "standard", "digital_badge"].map((v) => ({ value: v, label: v })),
        },
        { key: "description", label: "Description" },
        { key: "isActive", label: "Active", type: "checkbox" },
      ]}
    />
  );
}

export function AgeBandsPage() {
  return (
    <CrudResource
      title="Age Bands"
      subtitle="Eligibility windows used by every event category."
      api={opsApi.ageBands}
      columns={[
        { key: "code", label: "Code" },
        { key: "label", label: "Label" },
        { key: "minAge", label: "Min" },
        { key: "maxAge", label: "Max" },
        { key: "isActive", label: "Active", render: (r) => yesNo(r.isActive) },
      ]}
      fields={[
        { key: "code", label: "Code", hidden: true },
        { key: "label", label: "Label" },
        { key: "minAge", label: "Min age", type: "number" },
        { key: "maxAge", label: "Max age", type: "number" },
        { key: "isActive", label: "Active", type: "checkbox" },
      ]}
    />
  );
}

export function TaxonomyPage() {
  return (
    <CrudResource
      title="Taxonomy"
      subtitle="Categories and sub-categories (set a parent id to nest)."
      api={opsApi.categories}
      columns={[
        { key: "code", label: "Code" },
        { key: "name", label: "Name" },
        { key: "parentId", label: "Parent", render: (r) => (r.parentId ? "sub-category" : "top level") },
        { key: "isActive", label: "Active", render: (r) => yesNo(r.isActive) },
      ]}
      fields={[
        { key: "code", label: "Code", hidden: true },
        { key: "name", label: "Name" },
        { key: "parentId", label: "Parent category id (optional)" },
        { key: "isActive", label: "Active", type: "checkbox" },
      ]}
    />
  );
}

export function CouponsPage() {
  return (
    <CrudResource
      title="Coupons"
      subtitle="The discount pool that participants apply at checkout."
      api={opsApi.coupons}
      columns={[
        { key: "code", label: "Code" },
        { key: "discountType", label: "Type" },
        { key: "value", label: "Value" },
        { key: "scope", label: "Scope" },
        { key: "usedCount", label: "Used" },
        { key: "isActive", label: "Active", render: (r) => yesNo(r.isActive) },
      ]}
      fields={[
        { key: "code", label: "Code", hidden: true },
        {
          key: "discountType",
          label: "Discount type",
          type: "select",
          options: ["percent", "flat", "sponsored_100"].map((v) => ({ value: v, label: v })),
        },
        { key: "value", label: "Value (percent or ₹)", type: "number" },
        {
          key: "scope",
          label: "Scope",
          type: "select",
          options: ["global", "event", "category"].map((v) => ({ value: v, label: v })),
        },
        { key: "maxUses", label: "Max uses (0 = unlimited)", type: "number" },
        { key: "isActive", label: "Active", type: "checkbox" },
      ]}
    />
  );
}

export function HallsPage() {
  return (
    <CrudResource
      title="Halls"
      subtitle="Venue registry available to Event Admins."
      api={opsApi.halls}
      columns={[
        { key: "name", label: "Name" },
        { key: "city", label: "City" },
        { key: "capacity", label: "Capacity" },
        { key: "baseRate", label: "₹/day" },
        { key: "leadName", label: "Lead" },
        { key: "isActive", label: "Active", render: (r) => yesNo(r.isActive) },
      ]}
      fields={[
        { key: "name", label: "Name" },
        { key: "city", label: "City" },
        { key: "capacity", label: "Capacity", type: "number" },
        { key: "baseRate", label: "Base rate ₹/day", type: "number" },
        { key: "leadName", label: "Hall lead name" },
        { key: "leadContact", label: "Hall lead contact" },
        { key: "isActive", label: "Active", type: "checkbox" },
      ]}
    />
  );
}

export function JudgesPage() {
  return (
    <CrudResource
      title="Judges"
      subtitle="The judge pool Event Admins assign from."
      api={opsApi.judges}
      columns={[
        { key: "name", label: "Name" },
        { key: "expertise", label: "Expertise" },
        { key: "experienceYears", label: "Years" },
        { key: "affiliation", label: "Affiliation" },
        { key: "isVerified", label: "Verified", render: (r) => yesNo(r.isVerified) },
      ]}
      fields={[
        { key: "name", label: "Name" },
        { key: "expertise", label: "Primary expertise" },
        { key: "experienceYears", label: "Years of experience", type: "number" },
        { key: "affiliation", label: "Affiliated school/academy" },
        { key: "isVerified", label: "ID verified", type: "checkbox" },
      ]}
    />
  );
}

export function CrewPoolPage() {
  return (
    <CrudResource
      title="Crew"
      subtitle="Reusable crew pool with engagement cost, assignable to any event."
      api={opsApi.crew}
      columns={[
        { key: "name", label: "Name" },
        { key: "role", label: "Role" },
        { key: "cost", label: "Cost ₹" },
        { key: "contact", label: "Contact" },
        { key: "isActive", label: "Active", render: (r) => yesNo(r.isActive) },
      ]}
      fields={[
        { key: "name", label: "Name" },
        {
          key: "role",
          label: "Role",
          type: "select",
          options: ["Stage", "Registration", "Green Room", "AV", "Security", "Hospitality"].map((v) => ({ value: v, label: v })),
        },
        { key: "cost", label: "Engagement cost ₹", type: "number" },
        { key: "contact", label: "Contact" },
        { key: "isActive", label: "Active", type: "checkbox" },
      ]}
    />
  );
}

export function VendorsPage() {
  return (
    <CrudResource
      title="Vendors"
      subtitle="Vendor registry, assignable to events (their cost adds to event income)."
      api={opsApi.vendors}
      columns={[
        { key: "name", label: "Name" },
        { key: "serviceType", label: "Service" },
        { key: "city", label: "City" },
        { key: "contact", label: "Contact" },
        { key: "isActive", label: "Active", render: (r) => yesNo(r.isActive) },
      ]}
      fields={[
        { key: "name", label: "Name" },
        { key: "serviceType", label: "Service type" },
        { key: "city", label: "City" },
        { key: "contact", label: "Contact" },
        { key: "isActive", label: "Active", type: "checkbox" },
      ]}
    />
  );
}

export function SponsorsPage() {
  return (
    <CrudResource
      title="Sponsors"
      subtitle="Sponsor profiles and committed funding."
      api={opsApi.sponsors}
      columns={[
        { key: "organisation", label: "Organisation" },
        { key: "tier", label: "Tier" },
        { key: "contact", label: "Contact" },
        { key: "committedAmount", label: "Committed ₹" },
        { key: "scholarshipSlots", label: "Slots" },
      ]}
      fields={[
        { key: "organisation", label: "Organisation" },
        {
          key: "tier",
          label: "Tier",
          type: "select",
          options: ["platinum", "gold", "silver", "impact"].map((v) => ({ value: v, label: v })),
        },
        { key: "contact", label: "Contact" },
        { key: "committedAmount", label: "Committed amount ₹", type: "number" },
        { key: "scholarshipSlots", label: "Scholarship slots", type: "number" },
      ]}
    />
  );
}
