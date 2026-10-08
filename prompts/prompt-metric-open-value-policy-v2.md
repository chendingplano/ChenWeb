You review one row extracted from a technical standard. The row is a requirement whose value the
document leaves open. You answer two questions about it. Judge the source clause, not the row's
name: the extractor may have named something as a quantity that the clause never names.

The source clause is `row.context`. When `row.context` is only a heading, a section number or a
location, use `row.desc` and `row.threshold_or_target` instead.

## Question `kind`: what does the open value belong to?

object_quantity: a quantity of a thing, facility, vehicle, container, equipment, product,
material, sample or test, whose value is left to be declared by a supplier or manufacturer,
agreed between named parties, or set, sized or chosen according to another factor (throughput,
population, demand, site conditions) or by design.
Examples: 产品应标明额定功率 (rated power, to be declared); 设备应明确处理能力和停留时间 (capacity and
retention time, to be declared); 应根据进水量合理确定调节池容积 (tank volume, sized by inflow); 应根据服务
人口确定收集车辆数量 (number of vehicles, sized); 保温时间由供需双方协商确定 (holding time of a product
process, agreed); 试样数量由检测双方约定 (number of test samples, agreed).

activity_schedule: the open value is when, how often or how an activity is carried out
(collection, transport, cleaning, inspection, maintenance, publicity, drop-off), and the clause
only tells parties to agree, set or announce it. The duty is organizational.
Examples: 清运时间和频次由物业与清运单位约定 (agree the collection time and frequency); 设施检修计划应提前
公告 (announce the maintenance plan); 应公告投放时间、地点和方式 (announce drop-off time, place and method).

not_a_quantity: the row names a method, process, practice, feature, record or identity, not a
quantity (主体工艺, a treatment route, a management system, a species name, "has a lid").

## Question `named`: does the source clause itself name the row's quantity?

Yes when the clause contains a word that names a quantity of the thing the row is about (the
equipment, staff, facility, product, sample, test or activity that must have the value): an
amount, size, volume, capacity, duration, energy use, frequency, level, error, limit, count or
number of it (for example 车辆数量, 人数, 台数, 容积, 周期, 时间, 频次, 能耗, 强度, 能力, AQL, MPE).

Words that name a factor the value depends on do not count: in 应根据服务人口和垃圾的数量配备相应的车辆,
数量 is the amount of waste, not of the vehicles, so the answer is no.

No when the clause only says to provide, equip, staff, set up or do something suitable,
necessary or as required, and the quantity appears only in the row's name. Examples:
应根据服务人口和作业量配备相应的清扫车辆和保洁人员 (no number of vehicles or staff is named);
应按需要设置足够的收集容器; 运营单位应配备专职管理人员; 应根据处理规模配置相应的检测设备.
Contrast: 应根据服务人口确定收集车辆数量 names 数量 of the vehicles, so the answer is yes.
