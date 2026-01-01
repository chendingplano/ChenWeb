//////////////////////////////////////////////////////////
// Description:
//   It defines the form page to pick schedules 
// Created: 2025/12/02 by Chen Ding
//////////////////////////////////////////////////////////
export const SchedulePicker = {
    pageName:"schedule_picker",
    pageType:"form",
    storeName:"schedule_store",
    storeLabel:"Schedule",
    components:[
        {
            fieldName: 'first_name',
            label: 'First Name',
            type: "input",
            placeholder:"e.g., Enter your first name",
            required: true,
            helpText: 'Please enter your first name',
            error: ''
        },
        {
            fieldName: 'event_name',
            label: 'Event Name',
            type: "select",
            placeholder:"e.g., select your event",
            required: true,
            helpText: 'Please select your event from the list',
            options: ["Free Consultation", "Project Kickoff", "Tax Filing Status Review", "Problem Solving"],
            error: ''
        },
        {
            fieldName: 'urgency',
            label: 'Urgency Level',
            type: "select",
            placeholder:"Select urgency level",
            required: true,
            helpText: "Please select the urgency level",
            options: ["Low", "Medium", "High", "Critical"],
            error: ''
        },
        {
            fieldName: 'schedule_desc',
            label: 'Description',
            type: "textarea",
            placeholder:"Brief describe your needs",
            required: false,
            helpText: "Brief describe your needs. If you need free consultation, for instance, tell us about what you want to know.",
            fullWidth: true,
            error: ''
        },
        {
            fieldName: 'user_email',
            label: 'Email',
            type: "input",
            placeholder:"Please provide your email",
            required: false,
            helpText: "Please provide your email so we can reach you to confirm the schedule.",
            error: ''
        },
        {
            fieldName: 'cell_phone',
            label: 'Cell Phone Number',
            type: "input",
            placeholder:"Please provide your cell phone number",
            required: false,
            helpText: "Please provide your cell phone number for quick contact if needed.",
            error: ''
        },
        {
            fieldName: 'sales_lead',
            label: 'How you hear about us',
            type: "select",
            placeholder:"Please select how you hear about us",
            required: false,
            helpText: "Please select how you hear about us",
            options: ["Referred by Friend", "From Google", "From X", "From other social media", "Others"],
            error: ''
        },
        {
            fieldName: 'schedule',
            label: 'Schedule',
            type: "date_time_picker",
            placeholder:"",
            required: true,
            helpText: "Please select a time from this Date-and-Time picker.",
            width: 750,
            height: 420,
            fullWidth: true,
            error: ''
        },
    ],
    pageCheckers:
    [
        {
            type: 'mandatory_fields',
            field_names: ["contact_email", "cell_phone"],
            error_msg: 'Email and cell phone number cannot be both missing.'
        }
    ]
}