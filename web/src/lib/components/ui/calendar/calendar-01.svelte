<!-- Calendar.svelte -->
<script lang="ts">
    import { onMount } from 'svelte';

    // Props
    let { width = 300, height = 360 } = $props<{ width?: number; height?: number }>();

    let monthYearEl: HTMLDivElement | null = null;
    let prevButtonEl = $state<HTMLDivElement | null>(null);
    let nextButtonEl: HTMLDivElement | null = null;

    const months = [
        'January', 'February', 'March', 'April', 'May', 'June',
        'July', 'August', 'September', 'October', 'November', 'December'
    ];

    let currentDate = $state(new Date());
    const today = new Date();

    type DayInfo = {
        text:       string;
        className:  string;
        year:       number;
        month:      number;  
        day:        number;
    };

    let days = $state<DayInfo[]>([]);
    let selectedDate = $state<Date | null>(null);
    let selectedTime = $state<string | null>(null);   

    // Generate time slots: 8:30–11:30 AM, 1:00–5:00 PM (30-min intervals)
    const generateTimeSlots = () => {
        const slots: string[] = [];

        // 8:30 AM to 11:30 AM
        for (let h = 8; h <= 11; h++) {
            const startMin = h === 8 ? 30 : 0;
            const endMin = h === 11 ? 30 : 60;
            for (let m = startMin; m < endMin; m += 30) {
                slots.push(`${h}:${m.toString().padStart(2, '0')}`);
            }
        }

        // 1:00 PM to 5:00 PM → 13:00 to 17:00
        for (let h = 13; h <= 17; h++) {
            const endMin = h === 17 ? 0 : 60; // stop at 17:00 (5:00 PM)
            for (let m = 0; m < endMin; m += 30) {
                const displayH = h > 12 ? h - 12 : h;
                const period = h >= 12 ? 'PM' : 'AM';
                slots.push(`${displayH}:${m.toString().padStart(2, '0')} ${period}`);
            }
        }

        return slots;
    }

    const timeSlots = generateTimeSlots();

    function handleTimeClicked(time: string) {
        if (selectedDate) {
            selectedTime = time;
            // Optional: emit combined selection
            console.log('Selected:', selectedDate.toDateString(), time);
        }
    }

    // Helper to check if a given date is "today"
    const isToday = (y: number, m: number, d: number): boolean => {
        return (
            y === today.getFullYear() &&
            m === today.getMonth() &&
            d === today.getDate()
        );
    };

    // Helper to get day of week and decide className
    const getClassName = (y: number, m: number, d: number): string => {
        if (isToday(y, m, d)) {
            return 'today';
        }
        const dayOfWeek = new Date(y, m, d).getDay();
        if (dayOfWeek === 0 || dayOfWeek === 6) {
            return 'fade';
        }
        return '';
    };

    $effect(() => {
        const date = currentDate;
        const year = date.getFullYear();
        const month = date.getMonth();
        const firstDay = new Date(year, month, 1).getDay();
        const lastDay = new Date(year, month + 1, 0).getDate();

        if (monthYearEl) {
            monthYearEl.textContent = `${months[month]} ${year}`;
        }

        const result: DayInfo[] = [];


        // Previous month
        const prevMonthLastDay = new Date(year, month, 0).getDate();
        const prevMonthYear = month === 0 ? year - 1 : year;
        const prevMonthIndex = month === 0 ? 11 : month - 1;

        for (let i = firstDay; i > 0; i--) {
            const dayNum = prevMonthLastDay - i + 1;
            result.push(
                {
                    text: String(dayNum), 
                    className: 'fade',
                    year: prevMonthYear,
                    month: prevMonthIndex,
                    day: dayNum
                });
        }

        // Current month
        for (let i = 1; i <= lastDay; i++) {
            const className = getClassName(year, month, i);
            result.push(
                { 
                    text: String(i), 
                    className,
                    year: year,
                    month: month,
                    day: i
                });
        }

        // Next month
        const nextMonthStartDay = 7 - new Date(year, month + 1, 0).getDay() - 1;
        const nextMonthYear = month === 11 ? year + 1 : year;
        const nextMonthIndex = month === 11 ? 0 : month + 1;

        for (let i = 1; i <= nextMonthStartDay; i++) {
            const className = getClassName(nextMonthYear, nextMonthIndex, i);
            result.push(
                { 
                    text: String(i), 
                    className: ' fade',
                    year: nextMonthYear,
                    month: nextMonthIndex,
                    day: i
                });
        }

        days = result;
    });

    // Apply dimensions via CSS custom properties
    $effect(() => {
        if (monthYearEl?.parentElement?.parentElement) {
            const root = monthYearEl.parentElement.parentElement; // .calendar
            root.style.setProperty('--calendar-width', `${width}px`);
            root.style.setProperty('--calendar-height', `${height}px`);
        }
    });

    $effect(() => {
        prevButtonEl?.addEventListener('click', () => {
            // Prevent going to past months before current month
            if (currentDate.getFullYear() < today.getFullYear()) {
                return
            }

            if (currentDate.getFullYear() === today.getFullYear() &&
                currentDate.getMonth() <= today.getMonth()) {
                return
            }
            currentDate = new Date(currentDate.getFullYear(), currentDate.getMonth() - 1, 1);
        }) 
    });

    onMount(() => {
        // Initial render is handled by $derived + template
        prevButtonEl?.addEventListener('click', () => {
            // Prevent going to past months before current month
            if (currentDate.getFullYear() < today.getFullYear()) {
                return
            }

            if (currentDate.getFullYear() === today.getFullYear() &&
                currentDate.getMonth() <= today.getMonth()) {
                return
            }
            currentDate = new Date(currentDate.getFullYear(), currentDate.getMonth() - 1, 1);
        });

        nextButtonEl?.addEventListener('click', () => {
          currentDate = new Date(currentDate.getFullYear(), currentDate.getMonth() + 1, 1);
        });
    });

    function goPrev() {
        currentDate = new Date(currentDate.getFullYear(), currentDate.getMonth() - 1, 1);
    }

    function goNext() {
        currentDate = new Date(currentDate.getFullYear(), currentDate.getMonth() + 1, 1);
    }

    function handleDateClicked(e: MouseEvent, day: DayInfo) {
        e.preventDefault();
        selectedDate = new Date(day.year, day.month, day.day);
        const dayOfWeek = selectedDate.getDay();
        if (dayOfWeek === 0 || dayOfWeek === 6) {
            // Ignore clicks on faded days
            return;
        }
        selectedTime = null; // reset time when date changes
        const target = e.target as HTMLElement;
        if (target && target.textContent) {
            alert(`You clicked on date: ${day.year}-${day.month + 1}-${day.day}, className:${day.className}`); // month is 0-based
        }
    }   

</script>

<link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/font-awesome/6.5.2/css/all.min.css" />

<div class="calendar">
    <div class="header">
        {#if currentDate.getFullYear() > today.getFullYear() || 
            (currentDate.getFullYear() === today.getFullYear() && currentDate.getMonth() > today.getMonth()) }
            <div bind:this={prevButtonEl} class="btn"><i class="fa-solid fa-arrow-left"></i></div>
        {:else}
            <div class="btn fade"></div>
        {/if}
        <div bind:this={monthYearEl} id="month-year"></div>
        <div bind:this={nextButtonEl} class="btn"><i class="fa-solid fa-arrow-right"></i></div>
    </div>
    <div class="weekdays">
        <div>Sun</div>
        <div>Mon</div>
        <div>Tue</div>
        <div>Wed</div>
        <div>Thu</div>
        <div>Fri</div>
        <div>Sat</div>
    </div>
    <div class="days">
        {#each days as day}
            {#if day.className === '' || day.className === 'today'}
                <button onclick={(e) => handleDateClicked(e, day)} class={day.className}>
                    {day.text}
                </button>
            {:else}
                <button class={day.className}>
                    {day.text}
                </button>
            {/if}
        {/each}
    </div>

    <!-- Time Picker -->
    <div class="time-picker">
        <h3>Available Times</h3>
        <div class="time-slots">
            {#each timeSlots as slot}
                <button
                    onclick={() => handleTimeClicked(slot)}
                    class:disabled={!selectedDate}
                    disabled={!selectedDate}>
                    {slot}
                </button>
            {/each}
        </div>
    </div>
</div>

<style>
  :global(body) {
    font-family: Arial, sans-serif;
    display: grid;
    place-items: center;
    height: 100vh;
    margin: 0;
    background-color: #333333;
  }

  .calendar {
    --calendar-width: 300px;
    --calendar-height: 360px;

    width: var(--calendar-width);
    height: var(--calendar-height);
    background-color: #111111;
    border-radius: 10px;
    box-shadow: 0 0 30px #222222;
    overflow: hidden;
    color: white;
    display: flex;
    flex-direction: column;
  }

  .header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 10px 20px;
  }

  .btn {
    cursor: pointer;
  }

  #month-year {
    font-weight: bold;
    font-size: 20px;
  }

  .weekdays,
  .days {
    display: flex;
    flex-wrap: wrap;
  }

  .weekdays {
    padding: 0 20px;
    font-weight: bold;
  }

  .days {
    padding: 0 20px 20px 20px;
    flex: 1; /* Only .days should fill remaining space */
    display: flex;
    flex-wrap: wrap;
  }

  .weekdays div,
  .days button {
    width: calc(100% / 7);
    text-align: center;
    padding: 0.5em 0;
    border-radius: 5px;
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .days button {
    transition: background-color 0.3s;
    background-color: #222222;
    border-style: sold;
    border-color: #333333;
    border-width: 0.5px;
    font: inherit;
    color: inherit;
    cursor: pointer;
  }

  .days button:hover {
    background-color: white;
    color: orangered;
  }

  .days .today {
    background-color: orangered;
    color: white;
  }

  .days .fade {
    color: #555;
    background-color: black;
    border-style: solid;
    border-color: #333333;
    cursor: default;
  }

  /* Time picker Styles */
  .time-picker {
    background-color: #111111;
    border-radius: 1, 10px;
    box-shadow: 0 0 30px #222222;
    color: white;
    padding: 20px;
    width: 160px;
    display: flex;
    flex-direction: column;
  }

  .time-picker h3 {
    margin: 0 0 15px 0;
    font-size: 16px;
    text-align: center;
  }

  .time-slots {
    display: flex;
    flex-direction: column;
    gap: 8px;
    overflow-y: auto;
    flex: 1;
  }

  .time-slots button {
    background: none;
    border: 1px solid #555;
    color: white;
    padding: 8px;
    border-radius: 4px;
    cursor: pointer;
    text-align: center;
    font-size: 14px;
  }

  .time-slots button:hover:not(:disabled) {
    background-color: #333;
  }

  .time-slots button:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
</style>
