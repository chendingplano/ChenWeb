<!-- Calendar.svelte -->
<script lang="ts">
    import { onMount } from 'svelte';

    // Props
    let { width = 600, height = 400 } = $props<{ width?: number; height?: number }>();

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
                slots.push(`${h}:${m.toString().padStart(2, '0')} am`);
            }
        }

        // 1:00 PM to 5:00 PM → 13:00 to 17:00
        for (let h = 13; h <= 17; h++) {
            const endMin = h === 17 ? 0 : 60; // stop at 17:00 (5:00 PM)
            for (let m = 0; m < endMin; m += 30) {
                const displayH = h > 12 ? h - 12 : h;
                const period = h >= 12 ? 'pm' : 'am';
                slots.push(`${displayH}:${m.toString().padStart(2, '0')} ${period}`);
            }
        }

        return slots;
    };

    const timeSlots = generateTimeSlots();

    function handleDateClicked(event: Event, day: DayInfo) {
        event.preventDefault()
        selectedDate = new Date(day.year, day.month, day.day);
        selectedTime = null; // reset time when date changes
    }

    function handleTimeClicked(time: string) {
        if (selectedDate) {
            selectedTime = time;
            // Optional: emit combined selection
            alert('Selected:' + selectedDate.toDateString() + ", at:" + time);
        }
    }

    // --- Calendar logic (unchanged except DayInfo extension) ---
    const isToday = (y: number, m: number, d: number): boolean => {
        return (
            y === today.getFullYear() &&
            m === today.getMonth() &&
            d === today.getDate()
        );
    };

    const getClassName = (y: number, m: number, d: number): string => {
        if (isToday(y, m, d)) return 'today';
        const dayOfWeek = new Date(y, m, d).getDay();
        return dayOfWeek === 0 || dayOfWeek === 6 ? 'fade' : '';
    };

    $effect(() => {
        const year = currentDate.getFullYear();
        const month = currentDate.getMonth();
        const firstDay = new Date(year, month, 1).getDay();
        const lastDay = new Date(year, month + 1, 0).getDate();

      if (monthYearEl) {
          monthYearEl.textContent = `${months[month]} ${year}`;
      }

      const result: DayInfo[] = [];

      // Previous month
      const prevYear = month === 0 ? year - 1 : year;
      const prevMonth = month === 0 ? 11 : month - 1;
      const prevMonthLastDay = new Date(year, month, 0).getDate();
      for (let i = firstDay; i > 0; i--) {
          const dayNum = prevMonthLastDay - i + 1;
          result.push({
              text: String(dayNum),
              className: 'fade',
              year: prevYear,
              month: prevMonth,
              day: dayNum
          });
      }

      // Current month
      for (let i = 1; i <= lastDay; i++) {
          result.push({
              text: String(i),
              className: getClassName(year, month, i),
              year,
              month,
              day: i
          });
      }

      // Next month
      const nextYear = month === 11 ? year + 1 : year;
      const nextMonth = month === 11 ? 0 : month + 1;
      const nextMonthStartDay = 7 - new Date(year, month + 1, 0).getDay() - 1;
      for (let i = 1; i <= nextMonthStartDay; i++) {
          result.push({
              text: String(i),
              className: 'fade',
              year: nextYear,
              month: nextMonth,
              day: i
          });
      }

      days = result;
    });

    // Apply dimensions via CSS custom properties
    $effect(() => {
        if (monthYearEl?.parentElement?.parentElement?.parentElement) {
            const root = monthYearEl.parentElement.parentElement.parentElement; // .calendar
            console.log("Set calendar-width:", width)
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

    // Navigation
    onMount(() => {
        prevButtonEl?.addEventListener('click', () => {
            currentDate = new Date(currentDate.getFullYear(), currentDate.getMonth() - 1, 1);
            selectedDate = null
            selectedTime = null
        });

        nextButtonEl?.addEventListener('click', () => {
            currentDate = new Date(currentDate.getFullYear(), currentDate.getMonth() + 1, 1);
            selectedDate = null
            selectedTime = null
        });
    });
</script>

<link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/font-awesome/6.5.2/css/all.min.css" />

<div class="calendar-container">
    <!-- Calendar -->
    <div class="calendar">
        <div class="header">
            {#if currentDate.getFullYear() > today.getFullYear() || 
                (currentDate.getFullYear() === today.getFullYear() && currentDate.getMonth() > today.getMonth())}
              <div bind:this={prevButtonEl} class="header-btn"><i class="fa-solid fa-arrow-left"></i></div>
            {:else}
              <div bind:this={prevButtonEl} class="header-btn fade"></div>
            {/if}
            <div bind:this={monthYearEl} id="month-year"></div>
            <div bind:this={nextButtonEl} class="header-btn"><i class="fa-solid fa-arrow-right"></i></div>
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
                    <button
                        onclick={(e) => handleDateClicked(e, day)}
                        class={
                            selectedDate &&
                            selectedDate.getFullYear() === day.year &&
                            selectedDate.getMonth() === day.month &&
                            selectedDate.getDate() === day.day
                            ? 'selected' : day.className
                        }
                    >
                        {day.text}
                    </button>
                {:else}
                    <button class={day.className} disabled>
                        {day.text}
                    </button>
                {/if}
            {/each}
        </div>
    </div>

    <!-- Time Picker -->
    <div class="time-picker">
        <h3>Available Times</h3>
        <div class="time-slots">
            {#each timeSlots as slot}
                {#if selectedTime === slot}
                    <button
                      onclick={() => handleTimeClicked(slot)}
                      class="selected"
                    >
                      {slot}
                    </button>
                {:else}
                <button
                  onclick={() => handleTimeClicked(slot)}
                  class:disabled={!selectedDate}
                  disabled={!selectedDate}
                >
                  {slot}
                </button>
                {/if}
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

  .calendar-container {
    --calendar-width: 760px;
    --calendar-height: 460px;
    display: flex;
    gap: 20px;
    width: var(--calendar-width, 600px);
    height: var(--calendar-height, 400px);
  }

  .calendar {
    background-color: #111111;
    border-radius: 10px;
    box-shadow: 0 0 30px #222222;
    overflow: hidden;
    color: white;
    display: flex;
    flex-direction: column;
    flex: 1;
  }

  .header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 10px 20px;
  }

  .header-btn {
    cursor: pointer;
    width: 30px;
    text-align: center;
  }

  .fade {
    color: #555 !important;
    cursor: default !important;
  }

  #month-year {
    font-weight: bold;
    font-size: 20px;
  }

  .days {
    display: flex;
    flex-wrap: wrap;
    padding: 0 20px;
  }

  .weekdays {
    display: grid;
    grid-template-columns: repeat(7, 1fr);
    padding: 0 20px;
    align-content: center;
    font-weight: bold;
    margin-bottom: 8px;
  }

  .days {
    flex: 1;
    padding: 0 20px 20px 20px;
    display: grid;
    grid-template-columns: repeat(7, 1fr);
    grid-auto-rows: 1fr;
  }

  .days button {
    display: flex;
    align-items: center;
    justify-content: center;
    border: none;
    background: none;
    color: inherit;
    cursor: pointer;
    padding: 0.3em 0;
    border-radius: 5px;
    font: inherit;
  }

  .days button:disabled {
    cursor: default;
  }

  .days button:hover:not(:disabled) {
    background-color: white;
    color: orangered;
  }

  .days .selected,
  .time-slots .selected {
    background-color: white;
    color: orangered;
  }

  .days .today {
    background-color: orangered;
    color: white;
  }

  .days button {
    transition: background-color 0.3s;
    background-color: #333333;
    border: 1px solid #555;
    padding: 2px;
    font: inherit;
    color: inherit;
    cursor: pointer;
  }

  .days .fade {
    color: #555;
    background-color: black;
    border-style: solid;
    border-color: #333333;
    cursor: default;
  }

  /* Time Picker Styles */
  .time-picker {
    background-color: #111111;
    border-radius: 1, 10px;
    box-shadow: 0 0 30px #222222;
    color: white;
    padding: 20px;
    width: 220px;
    display: flex;
    flex-direction: column;
  }

  .time-picker h3 {
    margin: 0 0 15px 0;
    font-size: 16px;
    text-align: center;
  }

  .time-slots {
    display: grid;
    grid-template-columns: repeat(2, 1fr); 
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
    background-color: white;
    color: orangered
  }

  .time-slots button:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
</style>
